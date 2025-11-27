# --- Configuration Variables ---
# Name of the final executable file
APP_NAME = tesla-client
# The source file containing the main package
SRC_FILE = main.go

# Deployment Variables
# Target server's username
SSH_USER = oysteinl
# Target server's address (IP or hostname)
SSH_HOST = panda.localdomain
# Destination directory on the server
REMOTE_PATH = /home/oysteinl/tesla-client/

# --- Targets ---

.PHONY: all build deploy

# Default target: builds the application
all: build

## Build Target
# Compiles the Go application
build:
	@echo "Building $(APP_NAME)..."
	GOOS=linux GOARCH=amd64 go build -o $(APP_NAME) $(SRC_FILE)
	@echo "Build complete. Executable: $(APP_NAME)"

## Deploy Target
# Transfers the compiled application to the remote server and restarts the service (optional)
deploy: build
	@echo "Stopping tesla-client service..."
	ssh $(SSH_USER)@$(SSH_HOST) 'sudo /usr/sbin/service tesla-client stop'
	@echo "Copying $(APP_NAME) to $(SSH_USER)@$(SSH_HOST):$(REMOTE_PATH)..."
	scp $(APP_NAME) $(SSH_USER)@$(SSH_HOST):$(REMOTE_PATH)
	@echo "Starting tesla-client service..."
	ssh $(SSH_USER)@$(SSH_HOST) 'sudo /usr/sbin/service tesla-client start'

	@echo "Deployment complete."

