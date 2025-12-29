
Sui Go Example
This project is an example repository for interacting with the Sui blockchain using Go. It integrates the BlockRazor SDK wrapper and demonstrates how to perform development by calling the Sui RPC v2 interface.

📂 Project Structure
The core code is organized into the following main sections:

1. blockrzsdk/
This is the core SDK directory, containing high-level wrappers for Sui blockchain operations.

2. sui/rpc/v2/
This is the proto file for sui grpc support.

3. example/
This directory contains usage examples and serves as the best entry point for new users.

4. go.mod & go.sum
Go module files used for dependency management, defining the versions of third-party libraries required by the project.

🚀 Quick Start
Prerequisites
Go 1.24 or higher

Installation
Clone the repository

Bash

git clone https://github.com/BlockRazorinc/sui_go_example.git
cd sui_go_example
Download dependencies

Bash

go mod tidy
Running Examples
Navigate to the example directory to see specific use cases:

Bash

cd example
# Assuming the entry file is main.go
go run main.go

Note: Before running the examples, please check the code to see if you need to configure a Private Key or a specific RPC Node URL. It is recommended to use the Testnet environment for testing purposes.
