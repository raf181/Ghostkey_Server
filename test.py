import requests
import json
import time

# Configuration
ssid = "CASA-PONVI"  # Replace with your WiFi network name (SSID)
password = "Famili@_9once_Vivanco$"  # Replace with your WiFi password
api_host = "192.168.10.62"  # Replace with your API host address
api_port = 5000  # Replace with your API port
api_endpoint = "/get_command"  # Replace with your API endpoint for fetching commands
esp_id = "esp32_1"  # Replace with your ESP ID
esp_secret_key = "your_esp_secret_key"  # Replace with your ESP Secret Key

# Interval in seconds (e.g., 30 seconds)
api_check_interval = 30

# Function to fetch command from API
def fetch_command_from_api():
    url = f"http://{api_host}:{api_port}{api_endpoint}"
    params = {
        "esp_id": esp_id,
        "esp_secret_key": esp_secret_key
    }
    try:
        response = requests.get(url, params=params)
        response.raise_for_status()
        
        # Check if response contains JSON data
        if response.headers.get('content-type') == 'application/json':
            command_data = response.json()
            command = command_data.get("command", "").strip()
            if command:
                print(f"Fetched command from API: {command}")
                send_command_over_i2c(command)
            else:
                print("No command received from API")
        else:
            print(f"Unexpected response content: {response.content}")

    except requests.RequestException as e:
        print(f"Failed to connect to API: {e}")

    except json.JSONDecodeError as e:
        print(f"Failed to decode JSON response: {e}")

    except Exception as e:
        print(f"Error: {e}")

# Main loop to check for commands at intervals
def main():
    while True:
        fetch_command_from_api()
        time.sleep(api_check_interval)

if __name__ == "__main__":
    main()