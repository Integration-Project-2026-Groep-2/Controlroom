(service controlroom
 (exchange control_room_exchange direct)
 (queue heartbeat.mailing
  (routing_key routing.heartbeat.mailing)
  (schema      heartbeat_mailing.xsd)
  (index       heartbeat_mailing_logs)
  (dlq         heartbeat_mailing_dlq)))
