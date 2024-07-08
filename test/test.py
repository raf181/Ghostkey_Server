import requests
import json
import time

# Common configuration
ssid = "CASA-PONVI"  # Replace with your WiFi network name (SSID)
password = "Famili@_9once_Vivanco$"  # Replace with your WiFi password
api_host = "192.168.10.62"  # Replace with your API host address
api_port = 5000  # Replace with your API port
api_endpoint = "/get_command"  # Replace with your API endpoint for fetching commands

# Interval in seconds (e.g., 30 seconds)
api_check_interval = 30

def send_command_over_i2c(command):
    # Implementation of sending command over I2C should be here
    print(f"Sending command over I2C: {command}")
    # Example: i2c.write(command)
    pass

# Function to fetch command from API for a specific board
def fetch_command_from_api(esp_id, esp_secret_key):
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
                print(f"Fetched command from API for {esp_id}: {command}")
                send_command_over_i2c(command)
            else:
                print(f"No command received from API for {esp_id}")
        else:
            print(f"Unexpected response content for {esp_id}: {response.content}")

    except requests.RequestException as e:
        print(f"Failed to connect to API for {esp_id}: {e}")

    except json.JSONDecodeError as e:
        print(f"Failed to decode JSON response for {esp_id}: {e}")

    except Exception as e:
        print(f"Error for {esp_id}: {e}")

# Main function to load board credentials and fetch commands
def main():
    try:
        # Load board credentials from registered_boards.json
        with open('registered_boards.json', 'r') as f:
            boards = json.load(f)

        # Continuously fetch commands for all boards
        while True:
            for board in boards:
                fetch_command_from_api(board['esp_id'], board['esp_secret_key'])
            time.sleep(api_check_interval)

    except FileNotFoundError:
        print("Error: registered_boards.json not found. Make sure to run the registration script first.")

    except json.JSONDecodeError as e:
        print(f"Error decoding JSON from registered_boards.json: {e}")

if __name__ == "__main__":
    main()
