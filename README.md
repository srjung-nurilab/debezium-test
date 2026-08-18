# MongoDB CDC 개발 클러스터

MongoDB 7.0.8과 Debezium MongoDB connector가 읽을 수 있도록 3개 노드 replica set(`rs0`)을 Docker Compose로 구성합니다.

## 시작

```bash
docker compose up -d
docker compose ps
```

`mongo-init`이 종료되고 `mongo1`, `mongo2`, `mongo3`가 `healthy`이면 초기화가 끝난 상태입니다.

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
