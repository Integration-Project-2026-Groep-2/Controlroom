(service controlroom
 (exchange control_room_exchange direct)
 (queue heartbeat.planning
  (routing_key routing.heartbeat.planning)
  (schema      heartbeat_planning.xsd)
  (index       heartbeat_planning_logs)
  (dlq         heartbeat_planning_dlq)))
