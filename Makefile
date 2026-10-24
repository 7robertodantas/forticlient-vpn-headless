# Headless FortiClient VPN gateway. See README.md.
COMPOSE := docker compose -f docker/compose.yml
# Local settings (git-ignored): FCT_PROFILE, GW, SSH_OPTS. See .env.example.
-include .env
PROFILE ?= $(FCT_PROFILE)

.PHONY: docker-up docker-connect docker-status docker-disconnect docker-shell docker-down \
        vm-up vm-connect vm-status vm-disconnect vm-down \
        ssh-connect ssh-status ssh-disconnect

docker-up:
	$(COMPOSE) up -d --build
	@for i in $$(seq 1 60); do docker logs fct-headless 2>&1 | grep -q 'Ready\.' && break; sleep 1; done
	@docker logs fct-headless 2>&1

docker-connect:
	scripts/connect.sh "$(PROFILE)"

docker-status:
	docker exec fct-headless fct-status

docker-disconnect:
	docker exec fct-headless fct-disconnect "$(PROFILE)"

docker-shell:
	docker exec -it fct-headless bash

docker-down:
	$(COMPOSE) down --rmi local -v

VM := forticlient-vpn-headless
VM_EXEC := limactl shell --workdir / $(VM) sudo

vm-up:
	lima/up.sh

vm-connect:
	GW_EXEC="$(VM_EXEC)" scripts/connect.sh "$(PROFILE)"

vm-status:
	$(VM_EXEC) fct-status

vm-disconnect:
	$(VM_EXEC) fct-disconnect "$(PROFILE)"

vm-down:
	limactl delete -f $(VM)

# External VM gateway (reachable over SSH), set up with scripts/install-gateway.sh:
#   make ssh-connect GW=root@<gateway-ip> SSH_OPTS="-i ~/.ssh/<key>"
GW ?=
SSH_OPTS ?=
SSH_EXEC = ssh $(SSH_OPTS) $(GW)

ssh-connect:
	@test -n "$(GW)" || { echo "set GW=user@host"; exit 1; }
	GW_EXEC="$(SSH_EXEC)" scripts/connect.sh "$(PROFILE)"

ssh-status:
	$(SSH_EXEC) fct-status

ssh-disconnect:
	$(SSH_EXEC) "fct-disconnect '$(PROFILE)'"
