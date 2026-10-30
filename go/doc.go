// Package abertura é um cliente Go para as APIs de varejo da abertura do
// mercado de energia.
//
// Projeto independente e não oficial. Não é afiliado, patrocinado nem endossado pela Câmara de Comercialização de Energia Elétrica (CCEE). A descrição da API deriva da documentação pública e de testes no ambiente sandbox.
//
// # Dados pessoais em log
//
// Os tipos que carregam UC, e-mail, documento ou consumo (Migracao, Medicao e
// os tipos de entrada e filtro) imprimem só dados não pessoais em fmt e em slog.
// Com o handler JSON do slog, logue o id e não a lista inteira: uma lista passa
// por json.Marshal, que leva os campos em claro.
package abertura
