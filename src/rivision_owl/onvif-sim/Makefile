# ONVIF Simulator Makefile

.PHONY: build clean help

# 默认目标
build:
	go build -o ../build/onvif-sim .

# 交叉编译 RISC-V
build-riscv64:
	GOOS=linux GOARCH=riscv64 go build -o ../build/onvif-sim-riscv64 .

# 清理
clean:
	rm -f ../build/onvif-sim ../build/onvif-sim-riscv64

# 运行
run:
	go run . -verbose

help:
	@echo "ONVIF Simulator 编译"
	@echo ""
	@echo "目标:"
	@echo "  build        - 编译 x86_64"
	@echo "  build-riscv64 - 交叉编译 RISC-V"
	@echo "  run          - 直接运行"
	@echo "  clean        - 清理"
