package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/joho/godotenv"
)

// ================= CONFIGURAÇÃO =================
const (
	HARDCODED_INSTANCE = ""
	HARDCODED_TOKEN    = ""
	HARDCODED_CHAT     = ""
	API_URL            = "https://wh-bandeco-bot.onrender.com:10000/"
)

func getEnv(key, hardcoded string) string {
	if hardcoded != "" {
		return hardcoded
	}
	return os.Getenv(key)
}

func getInstance() string { return getEnv("GREEN_API_INSTANCE", HARDCODED_INSTANCE) }
func getToken() string    { return getEnv("GREEN_API_TOKEN", HARDCODED_TOKEN) }
func getChatId() string   { return getEnv("GREEN_API_CHAT", HARDCODED_CHAT) }

// ================= STRUCTS =================

type Refeicao struct {
	Padrao string `json:"padrao"`
	Vegano string `json:"vegano"`
}

type CardapioDia struct {
	Data   string   `json:"data"`
	Almoco Refeicao `json:"almoco"`
	Jantar Refeicao `json:"jantar"`
}

// ================= CARDÁPIO =================

func buscarCardapio(dataAlvo string) (*CardapioDia, error) {
	formData := url.Values{"data": {dataAlvo}}

	res, err := http.PostForm("https://www.sar.unicamp.br/RU/view/site/cardapio.php", formData)
	if err != nil {
		return nil, fmt.Errorf("erro ao fazer requisição: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return nil, fmt.Errorf("erro de status code: %d %s", res.StatusCode, res.Status)
	}

	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return nil, fmt.Errorf("erro ao carregar HTML: %v", err)
	}

	extrairRefeicao := func(seletor goquery.Selection) string {
		var cardapio []string
		seletor.Find("tr").Each(func(i int, s *goquery.Selection) {
			texto := strings.TrimSpace(s.Text())
			if texto != "" {
				linhaNormalizada := strings.Join(strings.Fields(texto), " ")
				cardapio = append(cardapio, linhaNormalizada)
			}
		})
		return strings.Join(cardapio, "\n")
	}

	dataDia := strings.TrimSpace(doc.Find("#dia .col-12.h3").Text())
	almocoPadrao := extrairRefeicao(*doc.Find("#normal .col-6").Eq(0).Find("table"))
	almocoVegano := extrairRefeicao(*doc.Find("#normal .col-6").Eq(1).Find("table"))
	jantarPadrao := extrairRefeicao(*doc.Find("#jantar .col-6").Eq(0).Find("table"))
	jantarVegano := extrairRefeicao(*doc.Find("#jantar .col-6").Eq(1).Find("table"))

	return &CardapioDia{
		Data:   dataDia,
		Almoco: Refeicao{Padrao: almocoPadrao, Vegano: almocoVegano},
		Jantar: Refeicao{Padrao: jantarPadrao, Vegano: jantarVegano},
	}, nil
}

func formatDate(t time.Time) string {
	return t.Format("2006-01-02")
}

func buscarSemana() ([]*CardapioDia, error) {
	var semana []*CardapioDia
	hoje := time.Now()
	for i := 0; i < 5; i++ {
		dia := hoje.AddDate(0, 0, i)
		cardapio, err := buscarCardapio(formatDate(dia))
		if err != nil {
			log.Println("Erro ao buscar dia:", formatDate(dia), err)
			continue
		}
		semana = append(semana, cardapio)
	}
	return semana, nil
}

func resumir(texto string) string {
	if len(texto) > 120 {
		return texto[:120] + "..."
	}
	return texto
}

func formatarSemana(semana []*CardapioDia) string {
	var msg strings.Builder
	msg.WriteString("📅 Cardápio da Semana\n\n")
	for _, dia := range semana {
		msg.WriteString(fmt.Sprintf(
			"📌 %s\n🍛 %s\n🥗 %s\n\n",
			dia.Data,
			resumir(dia.Almoco.Padrao),
			resumir(dia.Almoco.Vegano),
		))
	}
	return msg.String()
}

func formatarMensagem(cardapio *CardapioDia) string {
	return fmt.Sprintf(
		"🍽️ Cardápio RU - %s\n\n"+
			"🍛 ALMOÇO (Padrão):\n%s\n\n"+
			"🥗 ALMOÇO (Vegano):\n%s\n\n"+
			"🍝 JANTAR (Padrão):\n%s\n\n"+
			"🌱 JANTAR (Vegano):\n%s",
		cardapio.Data,
		cardapio.Almoco.Padrao,
		cardapio.Almoco.Vegano,
		cardapio.Jantar.Padrao,
		cardapio.Jantar.Vegano,
	)
}

// ================= WHATSAPP =================

func sendWhatsAppMessageTo(chatId, message string) {
	apiURL := fmt.Sprintf("%s/waInstance%s/sendMessage/%s", API_URL, getInstance(), getToken())

	if len(message) > 4000 {
		message = message[:4000]
	}

	payload := map[string]string{"chatId": chatId, "message": message}
	jsonData, _ := json.Marshal(payload)
	http.Post(apiURL, "application/json", bytes.NewBuffer(jsonData))
}

func sendWhatsAppMessage(message string) {
	sendWhatsAppMessageTo(getChatId(), message)
}

// ================= POLLING =================

type Notification struct {
	ReceiptId int                    `json:"receiptId"`
	Body      map[string]interface{} `json:"body"`
}

func receiveNotification() (*Notification, error) {
	apiURL := fmt.Sprintf("%s/waInstance%s/receiveNotification/%s", API_URL, getInstance(), getToken())

	resp, err := http.Get(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		var result Notification
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, nil
		}
		if result.ReceiptId == 0 {
			return nil, nil
		}
		return &result, nil
	}

	return nil, fmt.Errorf("status inesperado: %d", resp.StatusCode)
}

func deleteNotification(receiptId int) error {
	apiURL := fmt.Sprintf("%s/waInstance%s/deleteNotification/%s/%d",
		API_URL, getInstance(), getToken(), receiptId)

	req, err := http.NewRequest(http.MethodDelete, apiURL, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func processarNotificacao(notif *Notification) {
	body := notif.Body

	typeWebhook, _ := body["typeWebhook"].(string)
	if typeWebhook != "incomingMessageReceived" {
		return
	}

	senderData, ok := body["senderData"].(map[string]interface{})
	if !ok {
		return
	}
	chatId, _ := senderData["chatId"].(string)

	messageData, ok := body["messageData"].(map[string]interface{})
	if !ok {
		return
	}

	textData, ok := messageData["textMessageData"].(map[string]interface{})
	if !ok {
		return
	}

	text := strings.ToLower(textData["textMessage"].(string))
	log.Printf("Mensagem recebida de %s: %s", chatId, text)

	switch {
	case strings.Contains(text, "/ru hoje"):
		cardapio, err := buscarCardapio(formatDate(time.Now()))
		if err != nil {
			sendWhatsAppMessageTo(chatId, "Erro ao buscar cardápio de hoje 😕")
			return
		}
		sendWhatsAppMessageTo(chatId, formatarMensagem(cardapio))

	case strings.Contains(text, "/ru semanal"):
		semana, _ := buscarSemana()
		sendWhatsAppMessageTo(chatId, formatarSemana(semana))

	case strings.Contains(text, "/ru ajuda"):
		sendWhatsAppMessageTo(chatId,
			"🤖 Comandos disponíveis:\n\n"+
				"/ru hoje — cardápio do dia\n"+
				"/ru semanal — cardápio da semana",
		)
	}
}

func startPolling() {
	log.Println("Iniciando polling de mensagens...")
	for {
		notif, err := receiveNotification()
		if err != nil {
			log.Println("Erro ao receber notificação:", err)
			time.Sleep(5 * time.Second)
			continue
		}
		if notif == nil {
			continue
		}
		processarNotificacao(notif)
		if err := deleteNotification(notif.ReceiptId); err != nil {
			log.Println("Erro ao deletar notificação:", notif.ReceiptId, err)
		}
	}
}

// ================= MAIN =================

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("Arquivo .env não encontrado, usando variáveis de ambiente do sistema")
	}
	log.Println("Bot RU Unicamp iniciado!")

	// Polling roda em goroutine paralela
	go startPolling()

	// Servidor HTTP para o Render não matar o processo
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	log.Println("Servidor HTTP na porta", port, HARDCODED_CHAT, HARDCODED_INSTANCE, HARDCODED_TOKEN)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
