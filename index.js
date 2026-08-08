const axios = require('axios');
const cheerio = require('cheerio');

const BASE_URL = 'https://prefeituralimeira.unicamp.br';
const RESTAURANTES_URL = `${BASE_URL}/restaurantes-universitarios/`;
const DEFAULT_TIMEOUT = 15000;

const http = axios.create({
  timeout: DEFAULT_TIMEOUT,
  maxRedirects: 5,
  headers: {
    'User-Agent': 'wh-bandeco-bot/2.0 (+https://prefeituralimeira.unicamp.br/restaurantes-universitarios/)',
    Accept: 'text/html,application/xhtml+xml,application/pdf;q=0.9,*/*;q=0.8',
  },
});

function limparTexto(value) {
  return String(value || '')
    .replace(/\u00a0/g, ' ')
    .replace(/\s+/g, ' ')
    .trim();
}

function semAcentos(value) {
  return limparTexto(value)
    .normalize('NFD')
    .replace(/[\\u0300-\\u036f]/g, '')
    .toLowerCase();
}

function dataBR(data) {
  const date = new Date(`${data}T12:00:00`);
  if (Number.isNaN(date.getTime())) return data;
  return date.toLocaleDateString('pt-BR', {
    timeZone: 'America/Sao_Paulo',
    weekday: 'long',
    day: '2-digit',
    month: '2-digit',
    year: 'numeric',
  });
}

function ehLinkDeCardapio(href, texto) {
  const valor = semAcentos(`${href} ${texto}`);
  return /(cardapio|menu|refeicao|restaurante-universitario)/.test(valor);
}

function descobrirLinks(html, origem) {
  const $ = cheerio.load(html);
  const links = new Set([origem]);
  $('a[href]').each((_, element) => {
    const href = $(element).attr('href');
    const texto = $(element).text();
    if (!href || !ehLinkDeCardapio(href, texto)) return;
    try {
      const url = new URL(href, origem);
      if (url.protocol === 'http:' || url.protocol === 'https:') links.add(url.href);
    } catch (_) {}
  });
  return [...links];
}

function linhaParaRefeicao(linha) {
  const valores = linha.map(limparTexto).filter(Boolean);
  if (!valores.length) return null;
  const texto = valores.join(' | ');
  const tipo = semAcentos(texto);
  if (/^(almoco|almoço)\\b/.test(tipo)) return { tipo: 'almoco', texto };
  if (/^jantar\\b/.test(tipo)) return { tipo: 'jantar', texto };
  if (/^cafe|café/.test(tipo)) return { tipo: 'cafe', texto };
  return null;
}

function extrairTabelas($) {
  const refeicoes = { almoco: [], jantar: [], cafe: [] };
  $('table').each((_, table) => {
    $(table).find('tr').each((__, row) => {
      const celulas = $(row).find('th,td').map((___, cell) => $(cell).text()).get();
      const refeicao = linhaParaRefeicao(celulas);
      if (refeicao) refeicoes[refeicao.tipo].push(refeicao.texto);
    });
  });
  return refeicoes;
}

function extrairSecoes($) {
  const refeicoes = { almoco: [], jantar: [], cafe: [] };
  $('h1,h2,h3,h4,h5,p,li').each((_, element) => {
    const texto = limparTexto($(element).text());
    const tipo = semAcentos(texto);
    if (/^(almoco|almoço)\\b/.test(tipo)) refeicoes.almoco.push(texto);
    else if (/^jantar\\b/.test(tipo)) refeicoes.jantar.push(texto);
    else if (/^cafe|café/.test(tipo)) refeicoes.cafe.push(texto);
  });
  return refeicoes;
}

function mesclarRefeicoes(tabelas, secoes) {
  return Object.fromEntries(Object.keys(tabelas).map((tipo) => [
    tipo,
    [...new Set([...tabelas[tipo], ...secoes[tipo]])],
  ]));
}

async function buscarCardapio(dataAlvo) {
  if (!/^\\d{4}-\\d{2}-\\d{2}$/.test(dataAlvo)) {
    throw new Error('dataAlvo deve estar no formato YYYY-MM-DD');
  }

  const pagina = await http.get(RESTAURANTES_URL);
  const urls = descobrirLinks(pagina.data, RESTAURANTES_URL);
  const dataFormatada = dataBR(dataAlvo);
  const termosData = [dataAlvo, dataFormatada, dataFormatada.replace(/,.*$/, '')]
    .map(semAcentos);

  let melhorResultado = null;
  for (const url of urls) {
    try {
      const resposta = url === RESTAURANTES_URL ? pagina : await http.get(url);
      const $ = cheerio.load(resposta.data);
      const textoPagina = semAcentos($.root().text());
      const refeicoes = mesclarRefeicoes(extrairTabelas($), extrairSecoes($));
      const quantidade = Object.values(refeicoes).reduce((total, itens) => total + itens.length, 0);
      const encontrouData = termosData.some((termo) => textoPagina.includes(termo));
      if (quantidade > 0 && (!melhorResultado || encontrouData || quantidade > melhorResultado.quantidade)) {
        melhorResultado = { refeicoes, encontrouData, quantidade, fonte: url };
        if (encontrouData) break;
      }
    } catch (error) {
      console.warn(`Falha ao consultar ${url}: ${error.message}`);
    }
  }

  if (!melhorResultado) {
    throw new Error(`Nenhum cardápio estruturado encontrado para ${dataAlvo}`);
  }

  return {
    data: dataAlvo,
    dataFormatada,
    almoco: melhorResultado.refeicoes.almoco,
    jantar: melhorResultado.refeicoes.jantar,
    cafe: melhorResultado.refeicoes.cafe,
    fonte: melhorResultado.fonte,
    encontrouData: melhorResultado.encontrouData,
  };
}

module.exports = { buscarCardapio };

if (require.main === module) {
  const data = process.argv[2] || new Date().toISOString().slice(0, 10);
  buscarCardapio(data)
    .then((resultado) => console.log(JSON.stringify(resultado, null, 2)))
    .catch((error) => {
      console.error(error.message);
      process.exitCode = 1;
    });
}