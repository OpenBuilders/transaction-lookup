### Transactions Observer
## Structure
|Name of worker|Amount|Description|
|:-------------|:----:|:----------|
|**Blockchain**||
|Master-blocks observer|1|Observes TON new master-blocks generation|
|Master-blocks handler|1|Fetchs all shard-blocks for generated master-block|
|Shard-blocks handler|10|Fetchs & handles all TXs from shard-block|
|**Redis**||
|Redis sender|1|Sends notification (stream) about noticed wallets|
|Redis events handler|1|Handles new keys & expired TTL events|