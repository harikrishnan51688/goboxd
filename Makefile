.PHONY: build up down test test-all test-c test-cpp test-java test-js test-py-hello test-py-fs test-py-net test-py-loop test-py-tle test-verilog test-bash test-bash-isolation test-readyz load

URL = http://localhost:8000/run
URL_READY = http://localhost:8000/readyz

load:
	docker exec goboxd /app/load-tester -url http://localhost:8000/run -c 2000 -n 50000

build:
	docker compose build

up:
	docker compose up -d

down:
	docker compose down

test-all: test-readyz test-c test-cpp test-java test-verilog test-js test-py-hello test-py-fs test-py-net test-py-loop test-py-tle test-bash test-bash-isolation
	@echo "========================================"
	@echo "   All tests executed successfully!"
	@echo "========================================"

test: test-all

test-c:
	@echo "Running C test..."
	@RESPONSE=$$(curl -s -X POST $(URL) -H 'Content-Type: application/json' -d @tests/test_c.json); \
	STATUS=$$(echo $$RESPONSE | grep -o '"status":"[^"]*"' | head -n1); \
	if [ "$$STATUS" = '"status":"ok"' ]; then \
		echo "C Test PASSED"; \
	else \
		echo "C Test FAILED: $$RESPONSE"; \
		exit 1; \
	fi

test-verilog:
	@echo "Running Verilog test..."
	@RESPONSE=$$(curl -s -X POST $(URL) -H 'Content-Type: application/json' -d @tests/test_verilog.json); \
	STATUS=$$(echo $$RESPONSE | grep -o '"status":"[^"]*"' | head -n1); \
	if [ "$$STATUS" = '"status":"ok"' ]; then \
		echo "Verilog Test PASSED"; \
	else \
		echo "Verilog Test FAILED: $$RESPONSE"; \
		exit 1; \
	fi

test-cpp:
	@echo "Running C++ test..."
	@RESPONSE=$$(curl -s -X POST $(URL) -H 'Content-Type: application/json' -d @tests/test_cpp.json); \
	STATUS=$$(echo $$RESPONSE | grep -o '"status":"[^"]*"' | head -n1); \
	if [ "$$STATUS" = '"status":"ok"' ]; then \
		echo "C++ Test PASSED"; \
	else \
		echo "C++ Test FAILED: $$RESPONSE"; \
		exit 1; \
	fi

test-java:
	@echo "Running Java test..."
	@RESPONSE=$$(curl -s -X POST $(URL) -H 'Content-Type: application/json' -d @tests/test_java.json); \
	STATUS=$$(echo $$RESPONSE | grep -o '"status":"[^"]*"' | head -n1); \
	if [ "$$STATUS" = '"status":"ok"' ]; then \
		echo "Java Test PASSED"; \
	else \
		echo "Java Test FAILED: $$RESPONSE"; \
		exit 1; \
	fi

test-js:
	@echo "Running JavaScript test..."
	@RESPONSE=$$(curl -s -X POST $(URL) -H 'Content-Type: application/json' -d @tests/test_js.json); \
	STATUS=$$(echo $$RESPONSE | grep -o '"status":"[^"]*"' | head -n1); \
	if [ "$$STATUS" = '"status":"ok"' ]; then \
		echo "JavaScript Test PASSED"; \
	else \
		echo "JavaScript Test FAILED: $$RESPONSE"; \
		exit 1; \
	fi

test-py-hello:
	@echo "Running Python Hello test..."
	@RESPONSE=$$(curl -s -X POST $(URL) -H 'Content-Type: application/json' -d @tests/test_python.json); \
	STATUS=$$(echo $$RESPONSE | grep -o '"status":"[^"]*"' | head -n1); \
	if [ "$$STATUS" = '"status":"ok"' ]; then \
		echo "Python Hello Test PASSED"; \
	else \
		echo "Python Hello Test FAILED: $$RESPONSE"; \
		exit 1; \
	fi

test-py-fs:
	@echo "Running Python FS Isolation test..."
	@RESPONSE=$$(curl -s -X POST $(URL) -H 'Content-Type: application/json' -d @tests/test_fs_isolation.json); \
	STATUS=$$(echo $$RESPONSE | grep -o '"status":"[^"]*"' | head -n1); \
	if [ "$$STATUS" = '"status":"ok"' ]; then \
		echo "Python FS Isolation Test PASSED"; \
	else \
		echo "Python FS Isolation Test FAILED: $$RESPONSE"; \
		exit 1; \
	fi

test-py-net:
	@echo "Running Python Network Isolation test..."
	@RESPONSE=$$(curl -s -X POST $(URL) -H 'Content-Type: application/json' -d @tests/test_net_isolation.json); \
	STATUS=$$(echo $$RESPONSE | grep -o '"status":"[^"]*"' | head -n1); \
	if [ "$$STATUS" = '"status":"ok"' ]; then \
		echo "Python Network Isolation Test PASSED"; \
	else \
		echo "Python Network Isolation Test FAILED: $$RESPONSE"; \
		exit 1; \
	fi

test-py-loop:
	@echo "Running Python Infinite Loop TLE test..."
	@RESPONSE=$$(curl -s -X POST $(URL) -H 'Content-Type: application/json' -d @tests/test_python_loop.json); \
	STATUS=$$(echo $$RESPONSE | grep -o '"status":"[^"]*"' | head -n1); \
	if [ "$$STATUS" = '"status":"time_limit_exceeded"' ]; then \
		echo "Python Infinite Loop TLE Test PASSED (correctly identified time_limit_exceeded)"; \
	else \
		echo "Python Infinite Loop TLE Test FAILED: $$RESPONSE"; \
		exit 1; \
	fi

test-py-tle:
	@echo "Running Python Sleep TLE test..."
	@RESPONSE=$$(curl -s -X POST $(URL) -H 'Content-Type: application/json' -d @tests/test_tle.json); \
	STATUS=$$(echo $$RESPONSE | grep -o '"status":"[^"]*"' | head -n1); \
	if [ "$$STATUS" = '"status":"time_limit_exceeded"' ]; then \
		echo "Python Sleep TLE Test PASSED (correctly identified time_limit_exceeded)"; \
	else \
		echo "Python Sleep TLE Test FAILED: $$RESPONSE"; \
		exit 1; \
	fi

test-bash:
	@echo "Running Bash test..."
	@RESPONSE=$$(curl -s -X POST $(URL) -H 'Content-Type: application/json' -d @tests/test_bash.json); \
	STATUS=$$(echo $$RESPONSE | grep -o '"status":"[^"]*"' | head -n1); \
	if [ "$$STATUS" = '"status":"ok"' ]; then \
		echo "Bash Test PASSED"; \
	else \
		echo "Bash Test FAILED: $$RESPONSE"; \
		exit 1; \
	fi

test-bash-isolation:
	@echo "Running Bash Isolation test..."
	@RESPONSE=$$(curl -s -X POST $(URL) -H 'Content-Type: application/json' -d @tests/test_bash_isolation.json); \
	STATUS=$$(echo $$RESPONSE | grep -o '"status":"[^"]*"' | head -n1); \
	if [ "$$STATUS" = '"status":"ok"' ]; then \
		echo "Bash Isolation Test PASSED"; \
	else \
		echo "Bash Isolation Test FAILED: $$RESPONSE"; \
		exit 1; \
	fi

test-readyz:
	@echo "Running Readiness check /readyz..."
	@RESPONSE=$$(curl -s -w "\nHTTP CODE: %{http_code}\n" $(URL_READY)); \
	STATUS=$$(echo "$$RESPONSE" | grep -o '"status":"[^"]*"' | head -n1); \
	CODE=$$(echo "$$RESPONSE" | grep -o "HTTP CODE: [0-9]*" | head -n1); \
	if [ "$$STATUS" = '"status":"ok"' ] && [ "$$CODE" = 'HTTP CODE: 200' ]; then \
		echo "Readiness test PASSED"; \
	else \
		echo "Readiness test FAILED: $$RESPONSE"; \
		exit 1; \
	fi
