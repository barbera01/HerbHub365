terraform {
  backend "azurerm" {
    resource_group_name  = "HerbHub365-Backend-RG"
    storage_account_name = "tfstatebackendherbhub"
    container_name       = "tfstate"
    key                  = "core/"
  }
}

