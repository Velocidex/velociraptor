# Elastic ECS support

[Elastic Common
Schema](https://www.elastic.co/guide/en/ecs/current/index.html) is an
Elastic/Opensearch database schema which normalizes event data into
the database. It is commonly used within the Elastic/Opensearch
ecosystem. Certain log forwarding agents (e.g. Winlogbeats) transform
the raw event data into ECS formatted objects. This mostly requires
renaming fields and adding some enrichments.

Velociraptor can also produce ECS compatible objects using the
`Elastic.EventLogs.Sysmon` artifact which may be forwarded to an
Elastic database.

This document explains how to initialize your indexes to support
Elastic ECS uploads.

## Using Curl to upload the index patten

Although Elastic by default infers the schema based on the first
document uploaded (So called `dynamic mapping`) this should be avoided
because it often gets it wrong and then rejects further events that do
not comply with the guessed schema.

Therefore it is advisable to initialize the schema manually. The best
way is using an "index template". This automatically initializes the
schema for all indexes that match the template (see the
`index_patterns` field in the schema)

You can upload an index template with curl using a JSON template in
this repository:

```bash
curl -kn -X PUT "localhost:9200/_index_template/winlogbeat-1" -H 'Content-Type: application/json' -d'@winlogbeat_schema.json'
```

NOTE: This index template was obtained from winlogbeats using the
following command

```bash
winlogbeat export template --es.version 8.11.1
```

But the index was modfied to remove all fields with the `flattened`
type which is not supported in opensearch. Therefore this index
template is fully compatible with opensearch as well.

Once the index template is uploaded you can create an index an index
using this curl command:

```
curl -kn -X PUT "localhost:9200/_data_stream/winlogbeat-velo"
```

Note that the default template uses a data stream which is more
efficient for insertions but objects can not be deleted from the
index.

## Delete the index

To delete the entire index you can use the following curl command:

```
curl -kn -X DELETE "localhost:9200/_data_stream/winlogbeat-velo" -H 'Content-Type: application/json'
```

## Viewing the data

As data is inserted into the index, you can inspect it manually

```
curl -kn -X GET "localhost:9200/winlogbeat-velo/_search?pretty" | less
```

But normally you would use something like Kibana to interact with the
data.
