# Codebase Documentation

## Setup Scripts

### `scripts/setup.sh` & `docker/scripts/setup.sh`

1. **Purpose:**
   - These scripts automate the setup process for creating necessary resources like App Registration, Resource Group, Storage Account, and setting environment variables for Azure services.

2. **Dependencies:**
   - Requires Azure CLI (`az`) to be installed.
   - Assumes Azure CLI is configured with appropriate credentials.

3. **Troubleshooting:**
   - If Azure CLI commands fail, check if Azure CLI is installed and configured properly.
   - Verify that the required permissions are granted for creating resources and managing App Registrations.
   - Check for any network issues that might prevent communication with Azure services.

4. **Usage:**
   - Run the script in a shell environment, and follow the prompts.
   - The script sets up the Azure resources and displays important information like Client ID, Client Secret, and Subscription details.

5. **Note:**
   - Environment variables set by the script are valid only for the current session.

## Services

### `services/watering/water_manager.go`

1. **Purpose:**
   - This Go program monitors soil moisture levels via Prometheus metrics and publishes watering commands to RabbitMQ when plants need water.

2. **Dependencies:**
   - Requires access to a Prometheus server for metrics collection.
   - Requires access to a RabbitMQ service for message publishing.

3. **Troubleshooting:**
   - Ensure connectivity to both Prometheus and RabbitMQ services.
   - Verify the correctness of service credentials.
   - Check for errors related to metric collection and message publishing.

### `services/admin/app/main.go`

1. **Purpose:**
   - An application built with Fiber that interacts with RabbitMQ to send messages to a queue.

2. **Dependencies:**
   - Requires connection to a RabbitMQ instance.

3. **Troubleshooting:**
   - Verify the RabbitMQ connection details.
   - Check for errors related to message publishing.

## Infrastructure as Code (IaC)

### `IaC/backend.tf`, `IaC/providers.tf`, `IaC/main.tf`

1. **Purpose:**
   - Terraform configuration files for managing Azure resources.

2. **Dependencies:**
   - Requires Terraform to be installed.
   - Assumes Azure provider configuration is set up.

3. **Troubleshooting:**
   - Ensure Terraform is properly installed and initialized.
   - Verify Azure provider configuration.
   - Check for any errors during resource creation or updates.

### `IaC/modules/ResourceGroups`

1. **Purpose:**
   - Terraform module for managing Azure Resource Groups.

2. **Dependencies:**
   - Depends on Azure provider configuration.

3. **Troubleshooting:**
   - Check for errors related to the creation of the Resource Group.
   - Verify the input variables provided to the module.

### `IaC/modules/security`

1. **Purpose:**
   - Placeholder module for future security-related configurations.

2. **Dependencies:**
   - N/A

3. **Troubleshooting:**
   - N/A

## Support Tools

### `support-tools/busstop/main.go`

1. **Purpose:**
   - A simple HTTP server using Chi that triggers a function on a POST request.

2. **Dependencies:**
   - Requires Chi package.

3. **Troubleshooting:**
   - Ensure the server is running and accessible.
   - Check for errors related to function triggering.

---

This documentation provides an overview of the codebase, its components, purposes, dependencies, and potential troubleshooting steps. Always ensure proper setup and configuration to avoid issues during deployment and execution.