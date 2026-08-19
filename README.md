# MongoDB CDC 개발 클러스터

MongoDB 7.0.8 replica set(`rs0`), NATS 3개 노드 클러스터(`nats`), PostgreSQL 18을 Docker Compose로 구성합니다. NATS는 CDC 메시지의 안정적인 보관을 위해 JetStream을 활성화합니다.

## 시작

```bash
docker compose up -d
docker compose ps
```

`mongo-init`이 성공 종료되고 `mongo1`, `mongo2`, `mongo3`, `nats1`, `nats2`, `nats3`, `postgres`가 `healthy`, `nats-box`, `debezium`이 실행 중이면 초기화가 끝난 상태입니다.

## Gin API controller

주문 CRUD는 MongoDB repository를 사용합니다. API를 실행하기 전에 MongoDB Compose 서비스를 먼저 기동해야 합니다. migration state API는 다음 단계에서 PostgreSQL metadata table로 교체하기 전까지 in-memory service를 사용합니다.

```bash
go run ./cmd/api
```

호스트에서 실행할 때 기본 MongoDB URI는 `mongodb://localhost:27017/?directConnection=true`, database는 `app`입니다. 다른 값을 사용하려면 `MONGODB_URI`, `MONGODB_DATABASE` 환경 변수를 설정합니다.

기본 포트는 `8080`이며, 주문 endpoint는 `POST|GET /orders`, `GET|PUT|DELETE /orders/:id`입니다. 모든 쓰기 요청에는 `Idempotency-Key` header가 필요합니다.

```bash
go test ./...
```

## PostgreSQL 접속

개발용 PostgreSQL 접속 정보는 다음과 같습니다.

```text
postgresql://postgres:postgres@localhost:5432/app
```

Docker 네트워크 내부에서는 `postgres:5432`로 접속합니다. logical replication을 위해 `wal_level=logical`, `max_replication_slots=10`, `max_wal_senders=10`을 설정했습니다.

## NATS 접속

`cdc` Docker 네트워크 내부의 Debezium/NATS 클라이언트에서는 다음 서버 목록을 사용합니다.

```text
nats://nats1:4222,nats://nats2:4222,nats://nats3:4222
```

호스트에서는 다음 포트로 접속할 수 있습니다.

| 노드 | Client | Monitoring |
| --- | ---: | ---: |
| `nats1` | `4222` | `8222` |
| `nats2` | `4223` | `8223` |
| `nats3` | `4224` | `8224` |

NATS 클러스터 route 포트(`6222`)는 Docker 내부 네트워크에서만 사용합니다.

## Debezium CDC

Debezium Server 3.6은 MongoDB replica set의 Change Stream을 읽어 NATS JetStream의 `MONGO_CDC` stream으로 발행합니다. 대상 collection은 `app.orders`, `app.migration_markers`이며 subject는 `mongo-cdc.*`입니다.

초기 적재는 향후 bulk migrator가 수행하므로 Debezium은 `snapshot.mode=no_data`로 설정했습니다. 즉, Debezium이 시작된 뒤의 변경분만 CDC로 전송합니다. 재시작 시 이어서 읽을 수 있도록 offset은 `debezium-offsets` volume에 보관합니다.

MongoDB connector의 value는 Debezium envelope JSON이며, `after`와 `before` 필드는 Extended JSON 형식의 문자열입니다. 이후 Go CDC applier는 envelope을 먼저 역직렬화한 다음 해당 문자열을 다시 Extended JSON으로 해석해야 합니다. 생성·수정·삭제 operation은 각각 `op`의 `c`·`u`·`d`로 구분합니다.

`nats-box`는 NATS CLI를 실행하기 위한 상시 도구 컨테이너입니다. 초기화 작업은 수행하지 않습니다. API 테스트 과정에서 필요하면 이 컨테이너로 JetStream 상태를 확인하거나 `MONGO_CDC` stream을 직접 생성합니다. stream 정의는 file storage, 3 replicas, 최대 40 GiB, 72시간 보관 정책이며 [nats/streams/mongo-cdc.json](/home/srjung/debezium-test/nats/streams/mongo-cdc.json)에 있습니다.

상태와 수신 메시지는 다음처럼 확인합니다.

```bash
docker compose exec nats-box nats --server nats://nats1:4222 server report jetstream
docker compose exec nats-box nats --server nats://nats1:4222 stream add --config /config/mongo-cdc.json
docker compose logs -f debezium
docker compose exec nats-box nats --server nats://nats1:4222 stream info MONGO_CDC
```

노드별 NATS 설정은 [nats/nats1.conf](/home/srjung/debezium-test/nats/nats1.conf), [nats/nats2.conf](/home/srjung/debezium-test/nats/nats2.conf), [nats/nats3.conf](/home/srjung/debezium-test/nats/nats3.conf)에 두었고, 각 컨테이너에 읽기 전용으로 mount합니다.

클러스터 상태는 다음처럼 확인합니다.

```bash
curl http://localhost:8222/routez
curl http://localhost:8222/jsz
```

## 접속

Debezium처럼 `cdc` Docker 네트워크에 연결된 클라이언트에서는 다음 URI를 사용합니다.

```text
mongodb://mongo1:27017,mongo2:27017,mongo3:27017/?replicaSet=rs0
```

호스트에서 단일 노드에 직접 접속해 데이터를 확인할 때는 다음처럼 사용합니다.

```text
mongosh "mongodb://localhost:27017/?directConnection=true"
```

각 노드는 호스트의 `27017`, `27018`, `27019` 포트로 노출되어 있습니다. replica set discovery를 사용하는 외부 클라이언트는 Docker 내부 DNS를 해석할 수 있어야 하므로, 그런 클라이언트는 우선 `cdc` 네트워크에 연결하는 방식으로 구성합니다.

현재는 로컬 CDC 개발을 위해 MongoDB 인증을 비활성화했습니다. 운영 또는 공유 환경에서는 keyfile 인증과 별도 CDC 사용자를 추가해야 합니다.

## 확인

```bash
docker compose exec mongo1 mongosh --quiet --eval 'rs.status()'
docker compose exec mongo1 mongosh --quiet --eval 'db.getSiblingDB("app").events.insertOne({ message: "hello", createdAt: new Date() })'
```

## 종료

컨테이너만 중지하려면 다음을 실행합니다.

```bash
docker compose down
```

데이터까지 초기화하려면 다음을 실행합니다.

```bash
docker compose down -v
```
