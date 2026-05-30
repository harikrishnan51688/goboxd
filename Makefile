.PHONY: build up down test load

URL = http://localhost:8000

build:
	docker compose build

up:
	docker compose up -d

down:
	docker compose down

test:
	@./tests/run_integration_tests.sh $(URL)

load:
	docker exec goboxd /app/load-tester -url $(URL)/run -c 2000 -n 50000
