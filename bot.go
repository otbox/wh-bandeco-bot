package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/joho/godotenv"
)

// ================= CONFIGURAÇÃO =================
const (
	HARDCODED_INSTANCE       = ""
	HARDCODED_TOKEN          = ""
	HARDCODED_CHAT           = ""
	HARDCODED_OPENROUTER_KEY = ""
	API_URL                  = "https://api.green-api.com"
	// OPENROUTER_URL           = "https://openrouter.ai/api/v1/chat/completions"
	OPENROUTER_URL = "https://openrouter.ai/api/v1/chat/completions"
	// OPENROUTER_MODEL         = "arcee-ai/trinity-mini:free" // modelo gratuito
	// OPENROUTER_MODEL         = "arcee-ai/trinity-mini:free" // modelo gratuito
	OPENROUTER_MODEL = "arcee-ai/trinity-mini:free" // modelo gratuito
	// OPENROUTER_MODEL = "<p>arcee-ai/trinity-mini:free</p>" // modelo gratuito
)

var chatsPermitidos map[string]bool

func getEnv(key, hardcoded string) string {
	if hardcoded != "" {
		return hardcoded
	}
	return os.Getenv(key)
}

func getAllowedChats() map[string]bool {
	permitidos := make(map[string]bool)

	raw := os.Getenv("ALLOWED_CHATS")
	if raw == "" {
		log.Println("WARN: ALLOWED_CHATS vazio — nenhum chat permitido")
		return permitidos
	}

	for _, c := range strings.Split(raw, ",") {
		c = strings.TrimSpace(c)
		if c != "" {
			permitidos[c] = true
		}
	}

	return permitidos
}

func getInstance() string      { return getEnv("GREEN_API_INSTANCE", HARDCODED_INSTANCE) }
func getToken() string         { return getEnv("GREEN_API_TOKEN", HARDCODED_TOKEN) }
func getChatId() string        { return getEnv("GREEN_API_CHAT", HARDCODED_CHAT) }
func getOpenRouterKey() string { return getEnv("OPENROUTER_API_KEY", HARDCODED_OPENROUTER_KEY) }

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

	// Tenta os seletores mais prováveis para o jantar
	jantarPadrao := extrairRefeicao(*doc.Find("#jantar .col-6").Eq(0).Find("table"))
	jantarVegano := extrairRefeicao(*doc.Find("#jantar .col-6").Eq(1).Find("table"))

	// Fallback: tenta #vegetariano caso o site use outro id
	if jantarPadrao == "" {
		jantarPadrao = extrairRefeicao(*doc.Find("#vegetariano .col-6").Eq(0).Find("table"))
		jantarVegano = extrairRefeicao(*doc.Find("#vegetariano .col-6").Eq(1).Find("table"))
	}

	// Fallback 2: tenta pegar pelo índice geral de tabelas
	if jantarPadrao == "" {
		log.Println("WARN: jantar não encontrado pelos seletores normais, tentando fallback por índice")
		allSections := doc.Find(".col-6")
		if allSections.Length() >= 4 {
			jantarPadrao = extrairRefeicao(*allSections.Eq(2).Find("table"))
			jantarVegano = extrairRefeicao(*allSections.Eq(3).Find("table"))
		}
	}

	log.Printf("DEBUG cardápio - Data: %s | Almoço P: %d chars | Jantar P: %d chars",
		dataDia, len(almocoPadrao), len(jantarPadrao))

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
	if len(texto) > 300 {
		return texto[:300] + "..."
	}
	return texto
}

func formatarSemana(semana []*CardapioDia) string {
	var msg strings.Builder
	msg.WriteString("📅 Cardápio da Semana\n\n")
	for _, dia := range semana {
		msg.WriteString(fmt.Sprintf(
			"📌 %s\n"+
				"🍛 Almoço:\n%s\n\n"+
				"🥗 Vegano:\n%s\n\n"+
				"🍝 Jantar:\n%s\n\n"+
				"🌱 Vegano Jantar:\n%s\n\n",
			dia.Data,
			resumir(dia.Almoco.Padrao),
			resumir(dia.Almoco.Vegano),
			resumir(dia.Jantar.Padrao),
			resumir(dia.Jantar.Vegano),
		))
	}
	return msg.String()
}

func formatarMensagem(cardapio *CardapioDia) string {
	jantar := cardapio.Jantar.Padrao
	if jantar == "" {
		jantar = "Não disponível"
	}
	jantarVeg := cardapio.Jantar.Vegano
	if jantarVeg == "" {
		jantarVeg = "Não disponível"
	}

	return fmt.Sprintf(
		"🍽️ Cardápio RU - %s\n\n"+
			"🍛 ALMOÇO (Padrão):\n%s\n\n"+
			"🥗 ALMOÇO (Vegano):\n%s\n\n"+
			"🍝 JANTAR (Padrão):\n%s\n\n"+
			"🌱 JANTAR (Vegano):\n%s",
		cardapio.Data,
		cardapio.Almoco.Padrao,
		cardapio.Almoco.Vegano,
		jantar,
		jantarVeg,
	)
}

// ================= OPENROUTER =================

var diasSemana = map[string]time.Weekday{
	"domingo": time.Sunday,
	"segunda": time.Monday,
	"terça":   time.Tuesday,
	"terca":   time.Tuesday,
	"quarta":  time.Wednesday,
	"quinta":  time.Thursday,
	"sexta":   time.Friday,
	"sábado":  time.Saturday,
	"sabado":  time.Saturday,
}

func extrairDiaSemana(pergunta string) (time.Weekday, bool) {
	pergunta = strings.ToLower(pergunta)

	// ===== AMANHÃ =====
	if strings.Contains(pergunta, "amanhã") || strings.Contains(pergunta, "amanha") {
		return time.Now().AddDate(0, 0, 1).Weekday(), true
	}

	// ===== DIAS DA SEMANA =====
	for nome, dia := range diasSemana {
		if strings.Contains(pergunta, nome) {
			return dia, true
		}
	}

	return 0, false
}

func proximoDia(target time.Weekday) time.Time {
	hoje := time.Now()
	diff := int(target - hoje.Weekday())

	if diff < 0 {
		diff += 7
	}

	return hoje.AddDate(0, 0, diff)
}

func tipoRefeicao(pergunta string) string {
	if strings.Contains(pergunta, "jantar") || strings.Contains(pergunta, "janta") {
		return "jantar"
	} else if strings.Contains(pergunta, "jantar") || strings.Contains(pergunta, "janta") {
		return "almoco"
	}
	return "nenhum"
}

func perguntarOpenRouter(pergunta string) string {

	var reCmd = regexp.MustCompile(`(?i)^/ru\s+(\w+)\s+(almoco|almoço|janta|jantar)$`)

	if matches := reCmd.FindStringSubmatch(pergunta); len(matches) == 3 {

		dataStr := strings.ToLower(matches[1])
		refeicao := strings.ToLower(matches[2])

		var data time.Time

		// ===== HOJE =====
		if dataStr == "hoje" {
			data = time.Now()

			// ===== AMANHÃ =====
		} else if dataStr == "amanha" || dataStr == "amanhã" {
			data = time.Now().AddDate(0, 0, 1)

			// ===== DIA DA SEMANA =====
		} else if dia, ok := diasSemana[dataStr]; ok {
			data = proximoDia(dia)

		} else {
			return "Formato inválido. Use: /ru hoje almoço | /ru terça jantar"
		}

		cardapio, err := buscarCardapio(formatDate(data))
		if err != nil || cardapio == nil {
			return "Não consegui obter o cardápio."
		}

		// ===== RESPOSTA DIRETA =====
		if refeicao == "janta" || refeicao == "jantar" {
			return fmt.Sprintf(
				"🍝 Jantar de %s:\n%s\n\n🌱 Vegano:\n%s",
				cardapio.Data,
				cardapio.Jantar.Padrao,
				cardapio.Jantar.Vegano,
			)
		}
		if refeicao == "almoço" || refeicao == "almoco" {
			return fmt.Sprintf(
				"🍛 Almoço de %s:\n%s\n\n🌱 Vegano:\n%s",
				cardapio.Data,
				cardapio.Almoco.Padrao,
				cardapio.Almoco.Vegano,
			)
		}

		return fmt.Sprintf(
			"🍛 Almoço de %s:\n%s\n\n"+
				"🥗 Vegano:\n%s\n\n"+
				"🍝 Jantar:\n%s\n\n"+
				"🌱 Vegano Jantar:\n%s\n\n",
			cardapio.Data,
			cardapio.Almoco.Padrao,
			cardapio.Almoco.Vegano,
			cardapio.Jantar.Padrao,
			cardapio.Jantar.Vegano,
		)
	}

	// ===== OPENROUTER =====
	key := getOpenRouterKey()
	if key == "" {
		return "Desculpe, não consegui obter a informação no momento. Tente novamente mais tarde."
	}

	payload := map[string]interface{}{
		"model": OPENROUTER_MODEL,
		"messages": []map[string]string{
			{
				"role": "system",
				"content": "Você é um assistente do Restaurante Universitário da Unicamp (RU/Bandeco). " +
					"Se for assunto do RU, seja hiper caloroso e fofo. " +
					"Caso contrário, responda de forma curta, grossa e cheia de gírias.",
			},
			{
				"role":    "user",
				"content": pergunta,
			},
		},
	}

	jsonData, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", OPENROUTER_URL, bytes.NewBuffer(jsonData))
	if err != nil {
		log.Println("Erro ao criar request OpenRouter:", err)
		return "Erro ao consultar IA."
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("HTTP-Referer", "https://github.com/ru-bot")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Println("Erro ao chamar OpenRouter:", err)
		return "Erro ao consultar IA."
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "Erro ao processar resposta da IA."
	}

	choices, ok := result["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		log.Println("OpenRouter sem choices:", result)
		return "Cansei, não irei mais responder perguntas, apenas comandos."
	}

	msg := choices[0].(map[string]interface{})["message"].(map[string]interface{})
	return msg["content"].(string)
}

// ================= WHATSAPP =================

func sendWhatsAppMessageTo(chatId, message string) {
	apiURL := fmt.Sprintf("%s/waInstance%s/sendMessage/%s", API_URL, getInstance(), getToken())

	if len(message) > 4000 {
		message = message[:4000]
	}

	payload := map[string]string{"chatId": chatId, "message": message}
	jsonData, _ := json.Marshal(payload)
	resp, err := http.Post(apiURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		log.Println("Erro ao enviar mensagem:", err)
		return
	}
	defer resp.Body.Close()
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
	// jsonPretty, _ := json.MarshalIndent(notif.Body, "", "  ")
	// log.Println("[DEBUG] BODY COMPLETO:\n", string(jsonPretty))
	if typeWebhook != "incomingMessageReceived" {
		log.Printf("Notificação ignorada: %s", typeWebhook)
		return
	}

	senderData, ok := body["senderData"].(map[string]interface{})
	if !ok {
		return
	}
	chatId, _ := senderData["chatId"].(string)

	if len(chatsPermitidos) > 0 && !chatsPermitidos[chatId] {
		log.Println("Chat não autorizado:", chatId)
		return
	}

	messageData, ok := body["messageData"].(map[string]interface{})
	if !ok {
		return
	}

	textData, ok := messageData["textMessageData"].(map[string]interface{})
	if !ok {
		return
	}

	msg, ok := textData["textMessage"].(string)
	if !ok {
		return
	}
	text := strings.ToLower(strings.TrimSpace(msg))
	log.Printf("Mensagem recebida de %s: %s", chatId, text)

	switch {
	case strings.Contains(text, "/ru hoje"):
		cardapio, err := buscarCardapio(formatDate(time.Now()))
		if err != nil || cardapio.Almoco.Padrao == "" {
			log.Println("Cardápio hoje indisponível, consultando OpenRouter...")
			resposta := perguntarOpenRouter(
				"O cardápio do RU da Unicamp de hoje não está disponível no site. " +
					"Avise o usuário de forma simpática e sugira que ele acesse https://www.prefeitura.unicamp.br/servicos/restaurantes-universitarios/ para verificar.",
			)
			sendWhatsAppMessageTo(chatId, resposta)
			return
		}
		sendWhatsAppMessageTo(chatId, formatarMensagem(cardapio))

	case strings.Contains(text, "/ru semanal"):
		semana, _ := buscarSemana()
		if len(semana) == 0 {
			log.Println("Cardápio semanal indisponível, consultando OpenRouter...")
			resposta := perguntarOpenRouter(
				"O cardápio semanal do RU da Unicamp não está disponível no site agora. " +
					"Avise o usuário de forma simpática e sugira que ele acesse https://www.prefeitura.unicamp.br/servicos/restaurantes-universitarios/ para verificar.",
			)
			sendWhatsAppMessageTo(chatId, resposta)
			return
		}
		sendWhatsAppMessageTo(chatId, formatarSemana(semana))

	case strings.Contains(text, "/ru ajuda"):
		sendWhatsAppMessageTo(chatId,
			"🤖 Comandos disponíveis:\n\n"+
				"/ru hoje — cardápio do dia\n"+
				"/ru semanal — cardápio da semana",
		)

	default:
		// Qualquer outra mensagem que não seja comando vai para o OpenRouter
		if strings.HasPrefix(text, "/ru ") {

			// ===== DIA DA SEMANA =====
			if dia, ok := extrairDiaSemana(text); ok {

				data := proximoDia(dia)
				cardapio, err := buscarCardapio(formatDate(data))

				if err != nil || cardapio == nil {
					sendWhatsAppMessageTo(chatId, "Não consegui obter o cardápio desse dia.")
					return
				}

				tipo := tipoRefeicao(text)

				if tipo == "jantar" {
					sendWhatsAppMessageTo(chatId, fmt.Sprintf(
						"🍝 Jantar de %s:\n%s\n\n🌱 Vegano:\n%s",
						cardapio.Data,
						cardapio.Jantar.Padrao,
						cardapio.Jantar.Vegano,
					))
					return
				}

				sendWhatsAppMessageTo(chatId, fmt.Sprintf(
					"🍛 Almoço de %s:\n%s\n\n🌱 Vegano:\n%s",
					cardapio.Data,
					cardapio.Almoco.Padrao,
					cardapio.Almoco.Vegano,
				))
				return
			}

			// ===== FALLBACK IA =====
			resposta := perguntarOpenRouter(text)
			sendWhatsAppMessageTo(chatId, resposta)
		}

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
			time.Sleep(2 * time.Second) // <-- ESSENCIAL
			continue
		}

		time.Sleep(1 * time.Second)
		processarNotificacao(notif)
		if err := deleteNotification(notif.ReceiptId); err != nil {
			log.Println("Erro ao deletar notificação:", notif.ReceiptId, err)
		}
	}
}

// ================= CONFIGURAR INSTÂNCIA =================

func configurarInstancia() {
	apiURL := fmt.Sprintf("%s/waInstance%s/setSettings/%s", API_URL, getInstance(), getToken())

	payload := map[string]interface{}{
		"incomingWebhook": "yes",
		"outgoingWebhook": "no",
		"stateWebhook":    "no",
		"webhookUrl":      "",
	}

	jsonData, _ := json.Marshal(payload)
	resp, err := http.Post(apiURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		log.Println("Erro ao configurar instância:", err)
		return
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	log.Println("Configuração da instância:", result)
}

// ================= MAIN =================
func selfPing(port string) {
	// Espera 1 minuto para o servidor subir antes de começar
	time.Sleep(1 * time.Minute)

	// url := fmt.Sprintf("http://localhost:%s/", port)
	url := fmt.Sprintf("https://wh-bandeco-bot.onrender.com/")
	// log.Println("Iniciando self-ping a cada 10 minutos para:", url)

	for {
		resp, err := http.Get(url)
		if err != nil {
			log.Println("Self-ping falhou:", err)
		} else {
			resp.Body.Close()
			log.Println("Self-ping OK")
		}
		time.Sleep(10 * time.Minute)
	}
}

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("Arquivo .env não encontrado, usando variáveis de ambiente do sistema")
	}
	chatsPermitidos = getAllowedChats()
	configurarInstancia()
	log.Println("Bot RU Unicamp iniciado!")

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	go startPolling()
	// log.Println(perguntarOpenRouter("vai porra"))
	go selfPing(port) // ← self-ping em goroutine paralela

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	log.Println("Servidor HTTP na porta", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
