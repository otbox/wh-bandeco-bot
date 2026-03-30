const axios = require('axios');
const cheerio = require('cheerio');

async function buscarCardapio(dataAlvo) {
    try {
        // 1. Fazendo o POST para o site
        const FormData = require('form-data');
        const form = new FormData();
        // O formato esperado pelo site é YYYY-MM-DD
        form.append('data', dataAlvo);

        const response = await axios.post('https://www.sar.unicamp.br/RU/view/site/cardapio.php', form, {
            headers: form.getHeaders()
        });

        // 2. Carregando o HTML no Cheerio
        const $ = cheerio.load(response.data);

        // 3. Função auxiliar para extrair o cardápio de uma tabela específica
        const extrairRefeicao = (seletor) => {
            let cardapio = [];
            $(seletor).find('tr').each((index, element) => {
                // Pega o texto de cada linha, troca quebras de linha (<br>) por um espaço ou hífen
                let linha = $(element).text().trim().replace(/\s\s+/g, ' ');
                if (linha) cardapio.push(linha);
            });
            return cardapio.join('\n');
        };

        // 4. Extraindo as informações pelas classes e IDs do HTML
        const dataDia = $('#dia .col-12.h3').text().trim();

        // Almoço (ID normal) -> O HTML deles coloca o almoço dentro da div #normal
        const almocoPadrao = extrairRefeicao('#normal .col-6:nth-child(1) table');
        const almocoVegano = extrairRefeicao('#normal .col-6:nth-child(2) table');

        // Jantar (ID vegetariano) -> O HTML deles coloca o jantar dentro da div #vegetariano
        const jantarPadrao = extrairRefeicao('#vegetariano .col-6:nth-child(1) table');
        const jantarVegano = extrairRefeicao('#vegetariano .col-6:nth-child(2) table');

        // Retornando um objeto estruturado
        return {
            data: dataDia,
            almoco: {
                padrao: almocoPadrao,
                vegano: almocoVegano
            },
            jantar: {
                padrao: jantarPadrao,
                vegano: jantarVegano
            }
        };

    } catch (error) {
        console.error('Erro ao buscar o cardápio:', error);
        return null;
    }
}

// Testando a função com a data de exemplo que você enviou
function callApi() {
    let data = []
    buscarCardapio('2026-03-23').then(dados => {
        
        data.push(dados)
    }
    );

}
