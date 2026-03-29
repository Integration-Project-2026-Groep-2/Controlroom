(service controlroom
 (exchange control_room_exchange direct)
 (queue heartbeat.crm
  (routing_key routing.heartbeat.crm)
  (schema      heartbeat_crm.xsd)
  (index       heartbeat_crm_logs)
  (dlq         heartbeat_crm_dlq)))
