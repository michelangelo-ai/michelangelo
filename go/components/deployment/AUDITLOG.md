# Query Audit Logs
1. Browse to this link
   ```
    https://querybuilder-ea.uberinternal.com/r/q7chwgKih/edit
   ```
2. Edit the report query as follows
   ```
    SELECT
    FROM_UNIXTIME(hadoop_timestamp / 1000) as datetime,
    event.msg.namespace as namespace,
    event.msg.entity_type as entity_type,
    event.msg.entity_name AS entity_name,
    event.msg.procedure AS procedure,
    event.msg.user_email AS user_email,
    event.msg.source AS source,
    event.msg.headers AS headers,
    event.msg.request AS request,
    event.msg.response AS response,
    event.msg.error AS error,
    event.msg.ma_uuid AS ma_uuid,
    event.msg.runtime_env AS runtime_env,
    event.msg.crd_diff AS crd_diff,
    event.msg.previous_crd AS previous_crd

    From rawdata_user.kafka_hp_michelangelo_apiserver_audit_log_nodedup as event
    where datestr > '2023-08-08' and event.msg.entity_name = 'deployment' and event.msg.source = 'controller-mgr'
   ```
3. Click the "Run" button in the bottom right corner to get the logs
