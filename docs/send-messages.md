# Envio de mensagens

Este documento descreve os endpoints de envio em
`/whatsapp/instances/{instance}/messages` e o recurso `options.mentionAll`.

Todas as rotas exigem uma sessão Arandu no tenant configurado e uma role
autorizada para `ActionMessageSend`:

```http
Cookie: arandu_session=<host-session>
```

Rotas disponíveis:

```text
POST /whatsapp/instances/{instance}/messages/text
POST /whatsapp/instances/{instance}/messages/link
POST /whatsapp/instances/{instance}/messages/media
POST /whatsapp/instances/{instance}/messages/media/file
POST /whatsapp/instances/{instance}/messages/audio
POST /whatsapp/instances/{instance}/messages/audio/file
POST /whatsapp/instances/{instance}/messages/contact
POST /whatsapp/instances/{instance}/messages/location
POST /whatsapp/instances/{instance}/messages/reaction
```

A identidade vem da sessão Arandu e do Grant emitido pela política. Respostas JSON são recursos
Arandu e ficam sob a chave `data`.

## MessageOptions

`MessageOptions` é opcional. Quando `mentionAll` está ausente ou é `false`, o envio continua síncrono e retorna a mensagem persistida com `200 OK`.

```json
{
  "delay": 1000,
  "presence": "composing",
  "quotedMessageId": 123,
  "quotedMessage": {
    "keyId": "A5FDD9082F21LGHLKJLGB6C3FF6BFA6F",
    "keyRemoteJid": "120363000000000000@g.us",
    "keyFromMe": false,
    "messageType": "extendedTextMessage",
    "content": {}
  },
  "externalAttributes": {
    "requestId": "request-456"
  },
  "mentionAll": true
}
```

`delay`: inteiro opcional em milissegundos. Envios gerais de mensagem aceitam até `120000`. Envios de áudio do WhatsApp aceitam até `300000`.

`presence`: string opcional. Texto, link, mídia, contato e localização aceitam `composing`. Áudio/PTV aceita `recording`. Áudio do WhatsApp também aceita `paused`.

`quotedMessageId`: id interno opcional da mensagem a ser citada. A mensagem precisa pertencer à mesma instância.

`quotedMessage`: snapshot opcional da mensagem citada com `keyId`, `keyRemoteJid`, `messageType` e `content`.

`externalAttributes`: objeto opcional copiado para os metadados da mensagem persistida e para os webhooks de resultado assíncrono de `mentionAll`.

`mentionAll`: booleano opcional. Quando `true`, o destinatário precisa ser um JID de grupo e a mensagem é aceita para processamento assíncrono quando o tipo da mensagem protobuf do WhatsApp suporta `ContextInfo`.

## Bodies dos endpoints

### sendText

```http
POST /whatsapp/instances/beplus/messages/text
Content-Type: application/json
```

```json
{
  "number": "120363000000000000@g.us",
  "options": {
    "mentionAll": true,
    "presence": "composing",
    "delay": 1000
  },
  "textMessage": {
    "text": "Aviso importante para todos."
  }
}
```

Suporta `mentionAll`.

### sendLink

```http
POST /whatsapp/instances/beplus/messages/link
Content-Type: application/json
```

```json
{
  "number": "5531999999999",
  "options": {
    "presence": "composing"
  },
  "linkMessage": {
    "link": "https://example.com",
    "thumbnailUrl": "https://example.com/thumb.jpg",
    "title": "Example",
    "description": "Example link"
  }
}
```

Suporta `mentionAll`.

### sendMedia

```http
POST /whatsapp/instances/beplus/messages/media
Content-Type: application/json
```

```json
{
  "number": "5531999999999",
  "options": {
    "presence": "composing"
  },
  "mediaMessage": {
    "mediatype": "image",
    "fileName": "image.jpg",
    "caption": "Caption",
    "media": "https://example.com/image.jpg"
  }
}
```

`mediatype` aceita `image`, `document`, `video`, `audio` e `ptv`. Suporta `mentionAll`.

### sendMediaFile

```http
POST /whatsapp/instances/beplus/messages/media/file
Content-Type: multipart/form-data
```

Campos multipart:

```json
{
  "number": "5531999999999",
  "mediaType": "image",
  "caption": "Caption",
  "presence": "composing",
  "delay": "1200",
  "quotedMessageId": "123",
  "quotedMessage": "{\"keyId\":\"abc\",\"keyRemoteJid\":\"5531999999999@s.whatsapp.net\",\"messageType\":\"extendedTextMessage\",\"content\":{\"text\":\"quoted\"}}",
  "mentionAll": "false",
  "attachment": "<binary file>"
}
```

`attachment` é o campo do arquivo. `mediaType` aceita `image`, `document`, `video`, `audio` e `ptv`. Suporta `mentionAll`.

### sendWhatsAppAudio

```http
POST /whatsapp/instances/beplus/messages/audio
Content-Type: application/json
```

```json
{
  "number": "5531999999999",
  "options": {
    "presence": "recording"
  },
  "audioMessage": {
    "audio": "https://example.com/audio.mp3"
  }
}
```

Baixa o áudio, converte/prepara como áudio PTT do WhatsApp e envia um `audioMessage`. Suporta `mentionAll`.

### sendWhatsAppAudioFile

```http
POST /whatsapp/instances/beplus/messages/audio/file
Content-Type: multipart/form-data
```

Campos multipart:

```json
{
  "number": "5531999999999",
  "presence": "recording",
  "delay": "1200",
  "quotedMessageId": "123",
  "quotedMessage": "{\"keyId\":\"abc\",\"keyRemoteJid\":\"5531999999999@s.whatsapp.net\",\"messageType\":\"extendedTextMessage\",\"content\":{\"text\":\"quoted\"}}",
  "mentionAll": "false",
  "attachment": "<binary audio file>"
}
```

`attachment` é o campo do arquivo de áudio. Suporta `mentionAll`.

### sendContact

```http
POST /whatsapp/instances/beplus/messages/contact
Content-Type: application/json
```

```json
{
  "number": "5531999999999",
  "options": {
    "quotedMessageId": 123,
    "presence": "composing"
  },
  "contactMessage": [
    {
      "fullName": "BePlus",
      "wuid": "5531999999999@s.whatsapp.net",
      "phoneNumber": "+55 31 99999-9999",
      "organization": "BePlus",
      "vcard": "BEGIN:VCARD\nVERSION:3.0\nFN:BePlus\nTEL;type=CELL;waid=5531999999999:+55 31 99999-9999\nEND:VCARD"
    }
  ]
}
```

`contactMessage` aceita um ou mais contatos. Se `vcard` for omitido, o serviço gera um a partir de `fullName`, `wuid`, `phoneNumber` e `organization`. Suporta `mentionAll`.

### sendLocation

```http
POST /whatsapp/instances/beplus/messages/location
Content-Type: application/json
```

```json
{
  "number": "5531999999999",
  "options": {
    "presence": "composing"
  },
  "locationMessage": {
    "name": "Belo Horizonte",
    "address": "Minas Gerais",
    "url": "https://example.com/place",
    "latitude": -19.9212,
    "longitude": -43.9378
  }
}
```

Suporta `mentionAll`.

### sendReaction

```http
POST /whatsapp/instances/beplus/messages/reaction
Content-Type: application/json
```

```json
{
  "reactionMessage": {
    "key": {
      "remoteJid": "5531999999999@s.whatsapp.net",
      "fromMe": true,
      "id": "3EB0FDD9082F21A9AC3D"
    },
    "reaction": "ok"
  }
}
```

Não suporta `mentionAll` porque `ReactionMessage` aponta para uma mensagem existente e não tem um campo `ContextInfo.MentionedJID` válido. Se `options.mentionAll=true`, a API retorna `400 Bad Request` com o código `MENTION_ALL_NOT_SUPPORTED_FOR_MESSAGE_TYPE`.

## Respostas de sucesso

Envios síncronos:

```http
HTTP/1.1 200 OK
Content-Type: application/json
```

O corpo é `{"data": <mensagem persistida>}`. O webhook `send.message` é
disparado depois da persistência.

Envios assíncronos com `mentionAll`:

```http
HTTP/1.1 202 Accepted
Content-Type: application/json
```

```json
{
  "data": {
    "statusCode": 202,
    "status": "processing",
    "message": "The message was accepted and is being processed.",
    "processId": "019f4ec1-f9b1-7c33-a4ef-d47715cb29e4",
    "instanceName": "beplus"
  }
}
```

`202 Accepted` significa que o snapshot, o job de processamento e o job de
retenção foram confirmados na mesma transação do banco. O resultado final é
entregue pelo webhook existente `send.message` e correlacionado por
`processId`.

## Menção invisível

`mentionAll=true` menciona todos os participantes atuais do grupo preenchendo `ContextInfo.MentionedJID` do WhatsApp. O servidor não adiciona marcadores visíveis `@phone` ao texto, legendas, cartões de contato, localizações ou corpos de mídia.

Endpoints suportados:

```text
sendText
sendLink
sendMedia
sendMediaFile
sendWhatsAppAudio
sendWhatsAppAudioFile
sendContact
sendLocation
```

Endpoints não suportados:

```text
sendReaction
```

A lista de participantes é buscada quando o worker processa o job. Participantes que entrarem depois dessa busca não serão mencionados. Participantes que saírem durante o processamento ainda podem estar presentes na lista buscada.

## Resultado por webhook

O evento de webhook existente é reutilizado:

```text
send.message
```

Exemplo de sucesso:

```json
{
  "event": "send.message",
  "instance": {
    "id": 1,
    "name": "beplus",
    "connectionStatus": "online",
    "ownerJid": "5511999999999@s.whatsapp.net",
    "externalAttributes": {}
  },
  "data": {
    "processId": "019f4ec1-f9b1-7c33-a4ef-d47715cb29e4",
    "status": "sent",
    "mentionAll": true,
    "data": {
      "messageId": "3EB0FDD9082F21A9AC3D",
      "remoteJid": "120363000000000000@g.us",
      "participantCount": 84,
      "timestamp": "2026-07-07T15:00:00Z"
    },
    "externalAttributes": {
      "requestId": "request-456"
    }
  },
  "timestamp": "2026-07-07T15:00:01Z"
}
```

Exemplo de falha:

```json
{
  "event": "send.message",
  "instance": {
    "id": 1,
    "name": "beplus",
    "connectionStatus": "online",
    "ownerJid": "5511999999999@s.whatsapp.net",
    "externalAttributes": {}
  },
  "data": {
    "processId": "019f4ec1-f9b1-7c33-a4ef-d47715cb29e4",
    "status": "failed",
    "mentionAll": true,
    "error": {
      "code": "GROUP_MENTION_PROCESSING_FAILED",
      "message": "Nao foi possivel concluir o envio da mensagem para o grupo."
    },
    "externalAttributes": {
      "requestId": "request-456"
    }
  },
  "timestamp": "2026-07-07T15:00:01Z"
}
```

Códigos de erro implementados para webhooks assíncronos:

```text
INSTANCE_NOT_CONNECTED
GROUP_INFO_FETCH_FAILED
GROUP_HAS_NO_PARTICIPANTS
MESSAGE_SEND_FAILED
GROUP_MENTION_PROCESSING_FAILED
```

## Erros HTTP

O destinatário não é um grupo:

```json
{
  "data": {
    "statusCode": 400,
    "error": "bad-request",
    "code": "MENTION_ALL_REQUIRES_GROUP",
    "messages": [
      "mention all requires group"
    ]
  }
}
```

O tipo de mensagem não suporta `mentionAll`:

```json
{
  "data": {
    "statusCode": 400,
    "error": "bad-request",
    "code": "MENTION_ALL_NOT_SUPPORTED_FOR_MESSAGE_TYPE",
    "messages": [
      "mention all is not supported for message type"
    ]
  }
}
```

Outros erros de validação, autenticação, instância, mídia, upload, persistência e conexão com o WhatsApp mantêm o envelope padrão de erro da API.

## Comportamento da fila

Envios assíncronos usam a `DatabaseQueue` nativa do Hesape. O pacote persiste o
protobuf preparado e seus metadados em `whatsapp_message_jobs`; o payload da
fila contém somente `processId`, portanto mensagens maiores que o limite de 32
KiB da fila continuam seguras no snapshot SQL. A criação do snapshot, do job de
processamento e do job de retenção ocorre em uma única transação: todos existem
ou nenhum é aceito.

A aplicação registra o handler explicitamente com
`Module.RegisterJobHandlers` e executa a fila dedicada:

```bash
aru queue:work --queue=whatsapp-messages --workers=N
```

O worker reemite o Grant com o mesmo tenant e a mesma action
`whatsapp.message.send` gravados pelo Hesape. Cada job admite cinco tentativas
com backoff. O `messageId` é estável entre tentativas, o snapshot é removido
somente depois que mensagem persistida, webhook de resultado e conclusão são
confirmados; snapshot ausente é tratado como conclusão idempotente. Em uma
falha terminal, o pacote remove o snapshot somente depois de confirmar o job
durável do webhook de erro. Se essa confirmação falhar, o dead letter conserva
o snapshot para `queue:retry` até o limite de retenção. Ao expirar, o job nativo
de limpeza remove tanto o snapshot quanto o job principal, impedindo retenção
indefinida do conteúdo da mensagem.

## Configuração Arandu

O módulo não lê variáveis de ambiente. A aplicação fornece
`Config.Processing` (`ProcessingTimeout`, `GroupInfoTimeout`, `SendTimeout` e
`Retention`) e
`Config.Media` (limites, timeout, diretório temporário e caminhos de
`ffmpeg`/`ffprobe`). `Processing.Workers` e `Processing.QueueSize` são mantidos
apenas por compatibilidade e ignorados; concorrência pertence a
`aru queue:work --workers=N`. Os padrões são 60s para o processamento completo,
30s para lookup/envio e 30 dias para retenção recuperável.

## Fluxo de processamento

```text
1. O cliente envia uma mensagem compatível com options.mentionAll=true.
2. A API valida autenticação, instância, payload e destinatário.
3. A API confirma que o destinatário é um JID de grupo.
4. A API prepara o protobuf e cria um `processId` com `data.NewID`.
5. A API persiste o snapshot e os jobs de processamento e retenção da `DatabaseQueue` na mesma transação.
6. A API retorna HTTP 202 Accepted.
7. Um worker recarrega a instância e resolve a sessão WhatsApp conectada naquele momento.
8. O worker busca os participantes atuais do grupo.
9. Os JIDs dos participantes são deduplicados e adicionados a ContextInfo.MentionedJID.
10. O corpo visível original da mensagem é preservado.
11. A mensagem é enviada pelo whatsmeow.
12. A persistência, o webhook durável de resultado e a remoção do snapshot e do job de retenção são confirmados atomicamente.
```

## Limitações conhecidas

`mentionAll` funciona somente para JIDs de grupo com servidor `g.us`.

O servidor não adiciona marcadores visíveis `@phone`.

`sendReaction` rejeita `mentionAll` porque reações não carregam um `ContextInfo` válido no nível da mensagem.

Grupos muito grandes podem aumentar o tempo de processamento.

`202 Accepted` confirma apenas que o snapshot e os jobs duráveis foram gravados;
não confirma que o WhatsApp já enviou a mensagem.

O webhook é a fonte do resultado final.

Clientes WhatsApp podem exibir ou notificar menções invisíveis de formas diferentes.
