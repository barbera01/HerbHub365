# Codebase Documentation

## [scripts/setup.sh](./scripts/setup.sh) & [docker/scripts/setup.sh](./docker/scripts/setup.sh)

### Purpose:
These scripts automate the setup process for creating an Azure App Registration, setting up environment variables, and creating a Resource Group with a Storage Account.

### Dependencies:
- Azure CLI installed and configured.
- Azure Subscription.
- `az` CLI commands.
- Azure App Registration.
- Storage Account.

### Troubleshooting Steps:
1. **Login Issues:**
   - Check Azure CLI is installed and logged in.
   - Verify the Azure Subscription is active.

2. **App Registration Creation:**
   - Ensure the App Registration creation command executes successfully.
   - Check if the required permissions are granted.

3. **Environment Variables:**
   - Verify that the environment variables are correctly set.
   - Check if the values are populated correctly.

4. **Resource Group and Storage Account Creation:**
   - Ensure the Resource Group and Storage Account creation commands execute without errors.
   - Check if the specified location and SKU are valid.

## [services/vision/app/main.go](./services/vision/app/main.go)

### Purpose:
This Go application interacts with a RabbitMQ server to process messages related to time-lapse requests.

### Dependencies:
- RabbitMQ server running and accessible.
- Go environment.

### Troubleshooting Steps:
1. **RabbitMQ Connection:**
   - Verify the RabbitMQ URL, username, and password are correct.
   - Check the RabbitMQ server status and connectivity.

2. **Message Processing:**
   - Ensure the application can fetch messages from the queue.
   - Verify the payload format and decoding process.

3. **Image Capture:**
   - Check the `rpicam-still` command execution.
   - Verify the output directory and file naming.

## [services/data-collector/main.go](./services/data-collector/main.go)

### Purpose:
This Go application collects data from environmental sensors like temperature and moisture sensors.

### Dependencies:
- Environmental sensors modules (envtemp, moisture, soiltemp).
- Go environment.

### Troubleshooting Steps:
1. **Sensor Readings:**
   - Verify the sensor modules are functioning correctly.
   - Check the sensor data collection process.

## [support-tools/busstop/main.go](./support-tools/busstop/main.go)

### Purpose:
This Go application serves as a simple HTTP server to trigger a function on button click.

### Dependencies:
- Go environment.
- Chi router library.

### Troubleshooting Steps:
1. **Server Startup:**
   - Ensure the server starts without errors.
   - Check for any middleware issues.

2. **Function Triggering:**
   - Verify the function is triggered on the specified route.
   - Check the response to client requests.

## [IaC](./IaC) (Infrastructure as Code)

### Purpose:
These files define the infrastructure setup using Terraform for resource groups, storage accounts, and security configurations.

### Dependencies:
- Terraform installed.
- Azure Provider configured.

### Troubleshooting Steps:
1. **Backend Configuration:**
   - Verify the Azure backend configuration in `backend.tf`.
   - Check the provider version compatibility in `providers.tf`.

2. **Resource Group Setup:**
   - Ensure the resource group module setup in `main.tf` is correct.
   - Validate the location and name settings.

3. **Security Module:**
   - Verify the security module configuration in `modules/security/main.tf`.
   - Check variables in `variables.tf`.

4. **Output & Variables:**
   - Ensure the output and variable definitions are consistent in respective files.

This documentation provides an overview of the codebase, its purpose, dependencies, and troubleshooting steps for each component.