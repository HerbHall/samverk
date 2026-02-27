.PHONY: install build test lint run

install:
	@echo "No dependencies yet -- concept phase"

build:
	@echo "No build targets yet -- concept phase"

test:
	@echo "No tests yet -- concept phase"

lint:
	@echo "Linting markdown..."
	npx markdownlint-cli2 "**/*.md" "#node_modules"

run:
	@echo "No runnable targets yet -- concept phase"
