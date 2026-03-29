(service controlroom
 (exchange control_room_exchange direct)
 (queue heartbeat.frontend
  (routing_key routing.heartbeat.frontend)
  (schema      heartbeat_frontend.xsd)
  (index       heartbeat_frontend_logs)
  (dlq         heartbeat_frontend_dlq)))
