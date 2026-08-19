# MongoDB CDC 개발 클러스터

MongoDB 7.0.8 replica set(`rs0`)과 NATS 3개 노드 클러스터(`nats`)를 Docker Compose로 구성합니다. NATS는 CDC 메시지의 안정적인 보관을 위해 JetStream을 활성화합니다.

## 시작

```bash
docker compose up -d
docker compose ps
```

`mongo-init`이 종료되고 `mongo1`, `mongo2`, `mongo3`, `nats1`, `nats2`, `nats3`가 `healthy`이면 초기화가 끝난 상태입니다.

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
