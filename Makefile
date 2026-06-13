SERVICES := $(wildcard services/*)

.PHONY: lint test build typecheck

lint:
	@for s in $(SERVICES); do $(MAKE) -C $$s lint || exit 1; done
	@# $(MAKE) -C frontend lint   # uncomment once a frontend exists

test:
	@for s in $(SERVICES); do $(MAKE) -C $$s test || exit 1; done
	@# $(MAKE) -C frontend test

typecheck:
	@for s in $(SERVICES); do $(MAKE) -C $$s vet || exit 1; done
	@# $(MAKE) -C frontend typecheck

build:
	@for s in $(SERVICES); do $(MAKE) -C $$s build || exit 1; done
	@# $(MAKE) -C frontend build
