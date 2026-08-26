package docs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

func JSON(doc Document) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(doc); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func Markdown(doc Document) (string, error) {
	var buffer bytes.Buffer

	writeLine(&buffer, "# Webhooks")
	writeLine(&buffer, "")
	writeLine(&buffer, "Documentação técnica dos webhooks implementados no código executável atual.")
	writeLine(&buffer, "")

	writeLine(&buffer, "## Sumário")
	for _, section := range []string{
		"Visão geral",
		"Configuração",
		"Mapa de eventos",
		"Headers HTTP",
		"Envelope padrão",
		"Estrutura da instância",
		"Entrega e tratamento de erros",
		"Eventos",
		"Eventos não suportados ou ignorados",
	} {
		writeLine(&buffer, "- [%s](#%s)", section, anchor(section))
	}
	writeLine(&buffer, "")

	writeLine(&buffer, "## Visão geral")
	writeLine(&buffer, "")
	writeLine(&buffer, "Webhooks são requisições HTTP `POST` assíncronas enviadas pela aplicação para uma URL configurada pelo consumidor. Cada entrega contém um envelope comum com o nome externo do evento, a instância que originou o evento, os dados específicos de `data` e o `timestamp` de criação do webhook.")
	writeLine(&buffer, "")
	writeLine(&buffer, "Existem dois destinos possíveis. Com o `Config.Prefix` padrão, o webhook da instância é configurado por `PUT /whatsapp/instances/{instance}/webhook`; o dispatcher consulta essa configuração diretamente no banco e só cria entregas para eventos cujas flags estejam habilitadas em `events`. O webhook global é configurado por `Config.Webhooks.GlobalURL` e `Config.Webhooks.GlobalEnabled`; quando habilitado, recebe todos os eventos suportados, sem aplicar as flags da instância.")
	writeLine(&buffer, "")
	writeLine(&buffer, "Para cada destino habilitado, o módulo salva um snapshot imutável da URL, body e headers em `whatsapp_webhook_deliveries` e inclui, na mesma transação, um job que carrega somente o `deliveryId`. Se a instância não tiver webhook habilitado e o webhook global estiver desabilitado, o evento é descartado sem erro.")
	writeLine(&buffer, "")
	writeLine(&buffer, "As entregas usam a `DatabaseQueue` nativa do Hesape. A aplicação registra o handler com `Module.RegisterJobHandlers`, inclui `whatsapp.WebhookQueueName` entre as filas monitoradas por `jobs.NewModule` e executa `aru queue:work --queue=whatsapp-webhooks --workers=N`. Respostas HTTP `2xx` são sucesso; erros de rede, timeout e respostas não `2xx` retornam erro para retry com o mesmo `X-Arandu-Delivery-ID`. A ordem relativa entre eventos não é garantida.")
	writeLine(&buffer, "")
	writeLine(&buffer, "- Versão do documento: `%s`.", doc.Version)
	writeLine(&buffer, "- Eventos oficiais documentados: `%d`.", len(doc.Events))
	writeLine(&buffer, "- Pacote de constantes: `%s`.", doc.GeneratedFrom.ConstantsPackage)
	writeLine(&buffer, "- Dispatcher: `%s`.", doc.GeneratedFrom.Dispatcher)
	writeLine(&buffer, "- Versão auditada do whatsmeow: `%s`.", doc.GeneratedFrom.WhatsmeowVersion)
	writeLine(&buffer, "")
	writeLine(&buffer, "Os exemplos de eventos com campos estáticos são verificados com `hesape/jsonschema` v0.12. Nos eventos marcados com campos dinâmicos, a presença e o tipo raiz de `data` continuam verificados, mas o objeto não é fechado nem validado campo a campo: esse builder não representa objetos abertos, e fechar esses objetos rejeitaria propriedades legítimas vindas do whatsmeow.")
	writeLine(&buffer, "")

	writeLine(&buffer, "## Configuração")
	writeLine(&buffer, "")
	writeLine(&buffer, "### Configuração tipada")
	writeLine(&buffer, "")
	writeLine(&buffer, "O módulo não lê variáveis de ambiente. A aplicação host fornece:")
	writeLine(&buffer, "")
	writeLine(&buffer, "| Campo | Tipo | Padrão | Descrição |")
	writeLine(&buffer, "| --- | --- | --- | --- |")
	writeLine(&buffer, "| `Config.Webhooks.GlobalURL` | URL | vazio | URL HTTP ou HTTPS do webhook global. |")
	writeLine(&buffer, "| `Config.Webhooks.GlobalEnabled` | boolean | `false` | Habilita o webhook global; exige `GlobalURL`. |")
	writeLine(&buffer, "| `Config.Webhooks.SigningSecret` | string | vazio | Segredo HMAC com no mínimo 32 bytes; obrigatório antes de habilitar qualquer webhook. |")
	writeLine(&buffer, "| `Config.Webhooks.Retention` | duração | `720h` (30 dias) | Tempo máximo de retenção de snapshots, inclusive falhas; zero usa o padrão. |")
	writeLine(&buffer, "| `Config.Webhooks.Workers` | inteiro | ignorado | Obsoleto; configure `--workers=N` em `aru queue:work`. |")
	writeLine(&buffer, "| `Config.Webhooks.QueueSize` | inteiro | ignorado | Obsoleto; a fila nativa é durável no banco. |")
	writeLine(&buffer, "")
	writeLine(&buffer, "A aplicação host precisa aplicar também as migrations da `DatabaseQueue`, registrar o handler do módulo no worker e consumir a fila `whatsapp-webhooks`.")
	writeLine(&buffer, "")
	writeLine(&buffer, "### Webhook da instância")
	writeLine(&buffer, "")
	writeLine(&buffer, "Configurar ou atualizar:")
	writeLine(&buffer, "")
	writeLine(&buffer, "```http")
	writeLine(&buffer, "PUT /whatsapp/instances/beplus/webhook HTTP/1.1")
	writeLine(&buffer, "Cookie: arandu_session=<host-session>")
	writeLine(&buffer, "Content-Type: application/json")
	writeLine(&buffer, "```")
	writeLine(&buffer, "")
	writeLine(&buffer, "Consultar:")
	writeLine(&buffer, "")
	writeLine(&buffer, "```http")
	writeLine(&buffer, "GET /whatsapp/instances/beplus/webhook HTTP/1.1")
	writeLine(&buffer, "Cookie: arandu_session=<host-session>")
	writeLine(&buffer, "```")
	writeLine(&buffer, "")
	writeLine(&buffer, "As duas rotas autenticam pela sessão Arandu e exigem Grants emitidos pela política para, respectivamente, `ActionWebhookSet` e `ActionWebhookView`. Respostas de configuração usam o envelope Arandu:")
	if err := writeJSONBlock(&buffer, map[string]any{"data": instanceConfigExample(doc)}); err != nil {
		return "", err
	}
	writeLine(&buffer, "")
	writeLine(&buffer, "`url` precisa usar `http` ou `https` e ter no máximo 500 caracteres. `enabled` ausente assume `true` na criação/atualização. Quando `events` é omitido, as flags existentes são preservadas; quando `events` é `{}`, as flags são removidas. Campos desconhecidos em `events` são rejeitados.")
	writeLine(&buffer, "")

	writeLine(&buffer, "## Mapa de eventos")
	writeLine(&buffer, "")
	writeLine(&buffer, "| Flag | Evento externo | Descrição |")
	writeLine(&buffer, "| --- | --- | --- |")
	for _, event := range doc.Events {
		writeLine(&buffer, "| `%s` | `%s` | %s |", event.Flag, event.Name, escapeTable(event.Description))
	}
	writeLine(&buffer, "")

	writeLine(&buffer, "## Headers HTTP")
	writeLine(&buffer, "")
	writeLine(&buffer, "| Header | Exemplo | Descrição |")
	writeLine(&buffer, "| --- | --- | --- |")
	for _, header := range doc.Headers {
		writeLine(&buffer, "| `%s` | `%s` | %s |", header.Name, escapeCode(header.Value), escapeTable(header.Description))
	}
	writeLine(&buffer, "")
	writeLine(&buffer, "Exemplo de requisição recebida pelo consumidor:")
	writeLine(&buffer, "")
	writeLine(&buffer, "```http")
	writeLine(&buffer, "POST /webhooks/beplus HTTP/1.1")
	writeLine(&buffer, "Host: example.com")
	writeLine(&buffer, "Content-Type: application/json")
	writeLine(&buffer, "User-Agent: Arandu-WhatsApp/1.0")
	writeLine(&buffer, "x-request-id: 019f0000-0000-7000-8000-000000000000")
	writeLine(&buffer, "x-owner-jid: 5531999999999@s.whatsapp.net")
	writeLine(&buffer, "x-instance-name: beplus")
	writeLine(&buffer, "x-instance-id: 1")
	writeLine(&buffer, "x-webhook-event: messages.upsert")
	writeLine(&buffer, "X-Arandu-Delivery-ID: 019f0000-0000-7000-8000-000000000001")
	writeLine(&buffer, "X-Arandu-Timestamp: 1785876000")
	writeLine(&buffer, "X-Arandu-Signature: sha256=<hex>")
	writeLine(&buffer, "```")
	writeLine(&buffer, "")
	writeLine(&buffer, "`x-owner-jid` pode ser uma string vazia quando a instância ainda não estiver conectada ou quando o proprietário não estiver salvo no snapshot usado pelo dispatcher.")
	writeLine(&buffer, "")

	writeLine(&buffer, "## Envelope padrão")
	writeLine(&buffer, "")
	if err := writeJSONBlock(&buffer, map[string]any{
		"event":     "nome.do.evento",
		"instance":  sampleInstance(false),
		"data":      map[string]any{},
		"timestamp": "2026-07-04T18:00:00Z",
	}); err != nil {
		return "", err
	}
	writeLine(&buffer, "")
	writeLine(&buffer, "`event` é o nome externo do evento. `instance` contém o snapshot mínimo da instância responsável pelo evento. `data` contém os dados específicos de cada evento e pode ser objeto ou array. `timestamp` é gerado quando o envelope é criado, em RFC3339 UTC.")
	writeLine(&buffer, "")

	writeLine(&buffer, "## Estrutura da instância")
	writeLine(&buffer, "")
	if err := writeJSONBlock(&buffer, map[string]any{
		"id":                 1,
		"name":               "beplus",
		"connectionStatus":   "online",
		"ownerJid":           "5531999999999@s.whatsapp.net",
		"externalAttributes": map[string]any{},
	}); err != nil {
		return "", err
	}
	writeLine(&buffer, "")
	writeLine(&buffer, "`id` é o identificador numérico interno da instância. `name` é o nome usado nas rotas. `connectionStatus` usa os valores persistidos da conexão, como `offline`, `connecting`, `qr_code`, `pairing_code`, `pairing`, `online`, `reconnecting`, `disconnected`, `connection_timeout`, `logged_out`, `session_missing`, `stream_replaced`, `keepalive_timeout`, `client_outdated`, `temporary_ban` e `connection_error`. `ownerJid` é `string` ou `null` no body; no header `x-owner-jid`, o valor nulo vira string vazia. `externalAttributes` sempre é um objeto JSON; valores ausentes, `null` ou inválidos são serializados como `{}`.")
	writeLine(&buffer, "")

	writeLine(&buffer, "## Entrega e tratamento de erros")
	writeLine(&buffer, "")
	if err := writeJSONBlock(&buffer, map[string]any{
		"delivery": map[string]any{
			"method":            doc.Delivery.Method,
			"contentType":       "application/json",
			"successStatus":     "200-299",
			"timeoutSeconds":    15,
			"queue":             doc.Delivery.Queue,
			"job":               doc.Delivery.Job,
			"maxTries":          doc.Delivery.MaxTries,
			"backoff":           doc.Delivery.Backoff,
			"idempotencyHeader": doc.Delivery.IdempotencyHeader,
			"allowedSchemes":    doc.Delivery.AllowedWebhookSchemes,
			"ordering":          "not_guaranteed",
		},
	}); err != nil {
		return "", err
	}
	writeLine(&buffer, "")
	for _, item := range doc.ErrorHandling {
		writeLine(&buffer, "- %s", item)
	}
	for _, item := range doc.Ordering {
		writeLine(&buffer, "- %s", item)
	}
	for _, item := range doc.Security {
		writeLine(&buffer, "- %s", item)
	}
	writeLine(&buffer, "- A criação do snapshot e a inserção do job compartilham `data.Transaction`: ambos fazem commit ou ambos fazem rollback.")
	writeLine(&buffer, "- Uma entrega concluída mantém apenas o tombstone necessário para idempotência; URL, body, headers e resposta são apagados imediatamente.")
	writeLine(&buffer, "- Snapshots de qualquer estado expiram após `Config.Webhooks.Retention`; jobs órfãos terminam como no-op, portanto redrive manual só existe dentro dessa janela.")
	writeLine(&buffer, "- `Start` e `Close` não criam nem encerram workers de webhook; o ciclo de vida da fila pertence à aplicação host.")
	writeLine(&buffer, "")

	writeLine(&buffer, "## Eventos")
	writeLine(&buffer, "")
	for _, event := range doc.Events {
		if err := writeEvent(&buffer, event); err != nil {
			return "", err
		}
	}

	writeLine(&buffer, "## Eventos não suportados ou ignorados")
	writeLine(&buffer, "")
	writeLine(&buffer, "| Evento interno | Status | Motivo |")
	writeLine(&buffer, "| --- | --- | --- |")
	for _, event := range doc.IgnoredEvents {
		writeLine(&buffer, "| `%s` | `%s` | %s |", event.Name, event.Status, escapeTable(event.Description))
	}
	writeLine(&buffer, "")

	return strings.TrimRight(buffer.String(), "\n") + "\n", nil
}

func writeEvent(buffer *bytes.Buffer, event EventDoc) error {
	writeLine(buffer, "### `%s`", event.Name)
	writeLine(buffer, "")
	writeLine(buffer, "%s", event.Description)
	writeLine(buffer, "")
	writeLine(buffer, "**Flag:** `%s`", event.Flag)
	writeLine(buffer, "")
	writeLine(buffer, "**Eventos internos:** `%s`", strings.Join(event.InternalEvents, "`, `"))
	writeLine(buffer, "")
	writeLine(buffer, "**Persistência:** %s", event.Persistence)
	if event.RequiresPersistenceFlag != "" {
		writeLine(buffer, "")
		writeLine(buffer, "**Flag de persistência:** `%s`", event.RequiresPersistenceFlag)
	}
	writeLine(buffer, "")
	writeLine(buffer, "**Tipo de `data`:** `%s`", event.DataType)
	writeLine(buffer, "")
	writeLine(buffer, "**DTO/normalizador:** `%s`", event.DataSchema)
	writeLine(buffer, "")
	writeLine(buffer, "**Campos dinâmicos:** %s", yesNo(event.DynamicFields))
	writeLine(buffer, "")
	writeLine(buffer, "**Implementado em:** `%s`", strings.Join(event.ImplementedIn, "`, `"))
	writeLine(buffer, "")

	writeLine(buffer, "#### Requisição")
	writeLine(buffer, "")
	writeLine(buffer, "```http")
	writeLine(buffer, "POST /webhooks/beplus HTTP/1.1")
	writeLine(buffer, "Content-Type: application/json")
	writeLine(buffer, "x-webhook-event: %s", event.Name)
	writeLine(buffer, "```")
	writeLine(buffer, "")

	writeLine(buffer, "#### Body")
	if err := writeJSONBlock(buffer, event.Example); err != nil {
		return fmt.Errorf("write example for %s: %w", event.Name, err)
	}
	writeLine(buffer, "")

	writeLine(buffer, "#### Campos de `data`")
	writeLine(buffer, "")
	writeFieldBullets(buffer, event.Fields)
	writeLine(buffer, "")

	if len(event.PossibleValues) > 0 {
		writeLine(buffer, "#### Valores possíveis")
		writeLine(buffer, "")
		for _, possible := range event.PossibleValues {
			writeLine(buffer, "- `%s`: `%s`", possible.Field, strings.Join(possible.Values, "`, `"))
		}
		writeLine(buffer, "")
	}

	writeLine(buffer, "#### Observações")
	writeLine(buffer, "")
	if len(event.Notes) == 0 {
		writeLine(buffer, "- Sem observações adicionais.")
	} else {
		for _, note := range event.Notes {
			writeLine(buffer, "- %s", note)
		}
	}
	writeLine(buffer, "")
	return nil
}

func writeFieldBullets(buffer *bytes.Buffer, fields []Field) {
	for _, item := range fields {
		requirement := "opcional"
		if item.Required {
			requirement = "obrigatório"
		}
		nullability := "não aceita `null`"
		if item.Nullable {
			nullability = "aceita `null`"
		}
		line := fmt.Sprintf("- `%s`: `%s`, %s, %s. %s", item.Name, item.Type, requirement, nullability, item.Description)
		if len(item.Values) > 0 {
			line += fmt.Sprintf(" Valores possíveis: `%s`.", strings.Join(item.Values, "`, `"))
		}
		writeLine(buffer, "%s", line)
	}
}

func writeJSONBlock(buffer *bytes.Buffer, value any) error {
	example, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	writeLine(buffer, "")
	writeLine(buffer, "```json")
	writeLine(buffer, "%s", string(example))
	writeLine(buffer, "```")
	return nil
}

func instanceConfigExample(doc Document) map[string]any {
	events := make(map[string]bool, len(doc.Events))
	for _, event := range doc.Events {
		events[event.Flag] = true
	}
	return map[string]any{
		"url":     "https://example.com/webhooks/beplus",
		"enabled": true,
		"events":  events,
	}
}

func sampleInstance(ownerNull bool) map[string]any {
	var owner any = "5531999999999@s.whatsapp.net"
	if ownerNull {
		owner = nil
	}
	return map[string]any{
		"id":                 1,
		"name":               "beplus",
		"connectionStatus":   "online",
		"ownerJid":           owner,
		"externalAttributes": map[string]any{},
	}
}

func writeLine(buffer *bytes.Buffer, format string, args ...any) {
	if len(args) == 0 {
		buffer.WriteString(format)
	} else {
		buffer.WriteString(fmt.Sprintf(format, args...))
	}
	buffer.WriteByte('\n')
}

func anchor(value string) string {
	value = strings.ToLower(value)
	value = strings.ReplaceAll(value, " ", "-")
	return value
}

func escapeTable(value string) string {
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.ReplaceAll(value, "|", "\\|")
}

func escapeCode(value string) string {
	return strings.ReplaceAll(value, "`", "\\`")
}

func yesNo(value bool) string {
	if value {
		return "sim"
	}
	return "não"
}

func ValidateDocument(doc Document) error {
	if !sort.SliceIsSorted(doc.Events, func(i, j int) bool {
		return doc.Events[i].Name < doc.Events[j].Name
	}) {
		return fmt.Errorf("events must be sorted alphabetically by name")
	}

	names := map[string]struct{}{}
	flags := map[string]struct{}{}
	for _, event := range doc.Events {
		if event.Name == "" {
			return fmt.Errorf("event with empty name")
		}
		if event.Flag == "" {
			return fmt.Errorf("event %s has empty flag", event.Name)
		}
		if len(event.Fields) == 0 {
			return fmt.Errorf("event %s has no fields", event.Name)
		}
		if _, ok := names[event.Name]; ok {
			return fmt.Errorf("duplicate event name %s", event.Name)
		}
		names[event.Name] = struct{}{}
		if _, ok := flags[event.Flag]; ok {
			return fmt.Errorf("duplicate event flag %s", event.Flag)
		}
		flags[event.Flag] = struct{}{}
		exampleEvent, ok := event.Example["event"].(string)
		if !ok || exampleEvent != event.Name {
			return fmt.Errorf("event %s example has event=%v", event.Name, event.Example["event"])
		}
		if _, ok := event.Example["timestamp"].(string); !ok {
			return fmt.Errorf("event %s example has no timestamp", event.Name)
		}
		if err := validateStaticEventExample(event); err != nil {
			return err
		}
		instance, ok := event.Example["instance"].(map[string]any)
		if !ok {
			return fmt.Errorf("event %s example has invalid instance", event.Name)
		}
		if id, ok := instance["id"].(int); !ok || id <= 0 {
			return fmt.Errorf("event %s example has invalid numeric instance.id", event.Name)
		}
		external, ok := instance["externalAttributes"]
		if !ok || external == nil || reflect.TypeOf(external).Kind() != reflect.Map {
			return fmt.Errorf("event %s example has invalid externalAttributes", event.Name)
		}
		if _, err := json.Marshal(event.Example); err != nil {
			return fmt.Errorf("event %s example is not JSON serializable: %w", event.Name, err)
		}
	}
	return nil
}
