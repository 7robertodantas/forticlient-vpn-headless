# FortiClient VPN Headless — Lima VM Validation
# Ephemeral Linux VM for reproducible testing

.PHONY: all validate validate-quick validate-ssh validate-cleanup validate-recreate \
        provision list vm-status check-deps

VM_NAME := forticlient-vpn-headless
LIMA_YAML := lima/forticlient-vpn-headless.yaml

# ─── Full validation cycle ───────────────────────────────────────────
validate:
	@if ! command -v limactl &>/dev/null; then echo "ERROR: limactl not found"; echo "Install with: brew install lima"; exit 1; fi
	@echo "╔══════════════════════════════════════════════════════════════════╗"
	@echo "║  Full Validation Cycle                                          ║"
	@echo "║  VM: $(VM_NAME)  Arch: x86_64                                   ║"
	@echo "╚══════════════════════════════════════════════════════════════════╝"
	@bash scripts/validate.sh

# ─── Quick validation (reuse existing VM) ────────────────────────────
validate-quick:
	@if ! command -v limactl &>/dev/null; then echo "ERROR: limactl not found"; exit 1; fi
	@bash scripts/validate.sh --skip-create

# ─── Recreate and validate ───────────────────────────────────────────
validate-recreate:
	@if ! command -v limactl &>/dev/null; then echo "ERROR: limactl not found"; exit 1; fi
	@echo "Recreating VM for clean validation..."
	@limactl stop $(VM_NAME) 2>/dev/null || true
	@limactl delete $(VM_NAME) 2>/dev/null || true
	@sleep 2
	@bash scripts/validate.sh --recreate

# ─── SSH into running VM ─────────────────────────────────────────────
validate-ssh:
	@if ! command -v limactl &>/dev/null; then echo "ERROR: limactl not found"; exit 1; fi
	@limactl shell $(VM_NAME)

# ─── Destroy VM and all artifacts ────────────────────────────────────
validate-cleanup:
	@if ! command -v limactl &>/dev/null; then echo "ERROR: limactl not found"; exit 1; fi
	@echo "Stopping and deleting VM $(VM_NAME)..."
	@limactl stop $(VM_NAME) 2>/dev/null || true
	@sleep 2
	@limactl delete $(VM_NAME) 2>/dev/null || true
	@echo "Cleaning up shared directory..."
	@rm -rf /tmp/lima-forticlient
	@echo "Done."

# ─── List VMs ────────────────────────────────────────────────────────
list:
	@if ! command -v limactl &>/dev/null; then echo "ERROR: limactl not found"; exit 1; fi
	@limactl list

# ─── VM status ───────────────────────────────────────────────────────
vm-status:
	@if ! command -v limactl &>/dev/null; then echo "ERROR: limactl not found"; exit 1; fi
	@limactl list | grep $(VM_NAME) || echo "VM $(VM_NAME) not found"

# ─── Setup shared directory ──────────────────────────────────────────
provision:
	@if ! command -v limactl &>/dev/null; then echo "ERROR: limactl not found"; exit 1; fi
	@mkdir -p /tmp/lima-forticlient
	@cp ~/Downloads/forticlient_vpn/forticlient_vpn_7.4.3.5411_amd64.deb /tmp/lima-forticlient/ 2>/dev/null || echo "WARN: FortiClient .deb not found"
	@if [ -f ~/Documents/Sangoma_Corp_VPN_North_Linux_READY.xml ]; then \
		echo "Copying VPN profile to shared dir..."; \
		cp ~/Documents/Sangoma_Corp_VPN_North_Linux_READY.xml /tmp/lima-forticlient/; \
	fi
	@echo "Shared directory ready at /tmp/lima-forticlient/"
	@ls -lh /tmp/lima-forticlient/

# ─── Dependency check ────────────────────────────────────────────────
check-deps:
	@echo "Checking dependencies..."
	@if command -v limactl &>/dev/null; then \
		echo "  ✓ limactl: $$($(limactl version 2>/dev/null | head -1))"; \
	else \
		echo "  ✗ limactl: NOT FOUND (brew install lima)"; \
	fi
	@if [ -d ~/Downloads/forticlient_vpn ]; then \
		echo "  ✓ FortiClient downloads found:"; \
		ls -lh ~/Downloads/forticlient_vpn/*.deb ~/Downloads/forticlient_vpn/*.rpm 2>/dev/null; \
	else \
		echo "  ✗ ~/Downloads/forticlient_vpn/ not found"; \
	fi
	@if [ -f ~/Documents/Sangoma_Corp_VPN_North_Linux_READY.xml ]; then \
		echo "  ✓ VPN profile found"; \
	else \
		echo "  ✗ VPN profile not found"; \
	fi
	@echo ""
