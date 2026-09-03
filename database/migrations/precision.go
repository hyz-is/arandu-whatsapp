package migrations

// microsecondPrecision is the fractional-second precision every timestamp
// column is declared with.
//
// The Blueprint's own default is whole seconds, and whole seconds are not
// enough here: whatsapp_message_updates keys on date_time, so at that
// granularity two transitions of one message into the same status inside one
// second collide, and the second one is refused instead of recorded.
const microsecondPrecision = 6
