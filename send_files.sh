#!/bin/bash

# Configuration
SERVER_URL="http://localhost:5000"
USERNAME="your_username"
PASSWORD="your_password"
ESP_ID="your_esp_id"
DELIVERY_KEY="your_delivery_key"
ENCRYPTION_PASSWORD="your_encryption_password"
SOURCE_DIR="./files_to_send"  # Directory containing the files to send

# First, login and get the session cookie
echo "Logging in..."
curl --location "$SERVER_URL/login" \
  --header 'Content-Type: application/x-www-form-urlencoded' \
  --data-urlencode "username=$USERNAME" \
  --data-urlencode "password=$PASSWORD" \
  --cookie-jar cookies.txt

# Check if login was successful
if [ $? -ne 0 ]; then
    echo "Login failed!"
    exit 1
fi

# Function to send a single file
send_file() {
    local file="$1"
    echo "Sending file: $file"
    
    curl --location "$SERVER_URL/cargo_delivery" \
      --cookie cookies.txt \
      --form "file=@$file" \
      --form "esp_id=$ESP_ID" \
      --form "delivery_key=$DELIVERY_KEY" \
      --form "encryption_password=$ENCRYPTION_PASSWORD"
    
    echo -e "\n"
}

# Check if source directory exists
if [ ! -d "$SOURCE_DIR" ]; then
    echo "Source directory $SOURCE_DIR does not exist!"
    exit 1
fi

# Send all files in the directory
echo "Starting file transfer..."
find "$SOURCE_DIR" -type f -print0 | while IFS= read -r -d '' file; do
    send_file "$file"
done

 