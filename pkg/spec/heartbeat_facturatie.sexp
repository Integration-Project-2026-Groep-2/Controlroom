(service controlroom
 (exchange control_room_exchange direct)
 (queue heartbeat.facturatie
  (routing_key routing.heartbeat.facturatie)
  (schema      heartbeat_facturatie.xsd)
  (index       heartbeat_facturatie_logs)
  (dlq         heartbeat_facturatie_dlq)))
