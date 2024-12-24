#!/bin/bash

# Variables
app_name="HerbHub365"
resource_group="HerbHub365-Backend-RG"
storage_account="herbubbackendsa"
location="uksouth"

# Login
az login

# Create App Registration
app_id=$(az ad app create --display-name "$app_name" --query appId -o tsv)
sp_id=$(az ad sp create --id "$app_id" --query id -o tsv)
az role assignment create --assignee "$sp_id" --role Contributor --scope "/subscriptions/$(az account show --query id -o tsv)"

# Generate credentials
client_secret=$(az ad app credential reset --id "$app_id" --append --query password -o tsv)

# Set environment variables for the current session
export ARM_CLIENT_ID="$app_id"
export ARM_CLIENT_SECRET="$client_secret"
export ARM_SUBSCRIPTION_ID=$(az account show --query id -o tsv)
export ARM_TENANT_ID=$(az account show --query tenantId -o tsv)

# Create Resource Group and Storage Account
az group create --name "$resource_group" --location "$location"
az storage account create --name "$storage_account" --resource-group "$resource_group" --location "$location" --sku Standard_LRS
az storage container create --name "tfstate" --account-name "$storage_account" --public-access off

# Output values
echo "Storage Account: $storage_account"
echo "Container Name: tfstate"
echo "Client ID: $app_id"
echo "Client Secret: $client_secret"
echo "Subscription ID: $ARM_SUBSCRIPTION_ID"
echo "Tenant ID: $ARM_TENANT_ID"

# Display reminder for the user
echo "NOTE: The environment variables are set for this session only."
