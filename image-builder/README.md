# RKE2 CAPI image builder

## Description

This directory contains the scripts and configuration files to build the images used by the RKE2 CAPI provider. It relies on the [packer](https://www.packer.io/) tool to build the images.

We are using bash scripts to provision images with required dependencies, scripts for each platform are located in the `scripts` directory.

## Prerequisites

### Installing Packer

Follow the [official Packer installation guide](https://developer.hashicorp.com/packer/install) for your platform.

**Linux (apt):**
```bash
wget -O - https://apt.releases.hashicorp.com/gpg | sudo gpg --dearmor -o /usr/share/keyrings/hashicorp-archive-keyring.gpg
echo "deb [signed-by=/usr/share/keyrings/hashicorp-archive-keyring.gpg] https://apt.releases.hashicorp.com $(lsb_release -cs) main" | sudo tee /etc/apt/sources.list.d/hashicorp.list
sudo apt update && sudo apt install packer
```

**macOS (Homebrew):**
```bash
brew tap hashicorp/tap
brew install hashicorp/tap/packer
```

Verify the installation:
```bash
packer --version
```

### Installing Make

**Linux (apt):**
```bash
sudo apt update && sudo apt install make
```

**macOS (Homebrew):**
```bash
brew install make
```

Verify the installation:
```bash
make --version
```

### Environment Variables
Note: RKE2_VERSION must not include the v prefix
```bash
export AWS_ACCESS_KEY_ID=<AWS access key>
export AWS_SECRET_ACCESS_KEY=<AWS secret access key>
export RKE2_VERSION=1.32.4+rke2r1
```

### Config opensuse-leap-160.json

Update the following attributes in [opensuse-leap-160.json](./aws/opensuse-leap-160.json)     
```json
"ssh_keypair_name": "",
"ssh_private_key_file": ""
```

## AWS

### Requirements

- Your AWS account must have the following permissions: https://developer.hashicorp.com/packer/plugins/builders/amazon#iam-task-or-instance-role

- You must a default VPC in your AWS account. If you don't have one, you can create one using the following command:

```bash
aws ec2 create-default-vpc
```

### Steps

For building the AWS AMIs, you can run the following command, this command will build a private openSUSE AMI:

```bash
make build-aws-opensuse-leap-160 RKE2_VERSION=${RKE2_VERSION}
```
or 

```bash
make help
```
and it will show you the available options.
