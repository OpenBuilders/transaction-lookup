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

## Environment

|Name of variable|Value|
|:---------------|:---:|
|Redis variables||
|`REDIS_HOST`|hostname:port|
|`REDIS_USER`|username|
|`REDIS_PASS`|password|
|`REDIS_DB`|db index, default - **0**|
|Liteserver variables||
|`LITESERVER_HOST`|ip:port|
|`LITESERVER_KEY`|key|
|`IS_TESTNET`|**true** for testnet, **false** for mainnet|
|`IS_PUBLIC`|**true** for public node else **false**|