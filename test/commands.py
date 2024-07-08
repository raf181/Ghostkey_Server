import requests
import json
import random
import time
import re

# Configuration
server_url = 'http://192.168.10.62:5000'
cookies_file = 'cookies.txt'  # Adjust path if necessary
commands = [
    "command1",
    "command2",
    "command3",
    # Add more commands as needed
]

# Function to load cookies from cookies.txt using regular expressions
def load_cookies(cookies_file):
    try:
        with open(cookies_file, 'r') as f:
            cookies_content = f.read()
            cookies = dict(re.findall(r'([^=\s]*)=([^;\n]*)', cookies_content))
        return cookies
    except FileNotFoundError:
        print(f"Error: {cookies_file} not found.")
        return {}
    except Exception as e:
        print(f"Error loading cookies from {cookies_file}: {e}")
        return {}

# Function to load registered boards from JSON file
def load_registered_boards(file):
    try:
        with open(file, 'r') as f:
            registered_boards = json.load(f)
        return registered_boards
    except FileNotFoundError:
        print(f"Error: {file} not found.")
        return []
    except json.JSONDecodeError as e:
        print(f"Error decoding JSON from {file}: {e}")
        return []

# Function to send random command to a random board
def send_random_command(server_url, cookies_file, boards):
    try:
        # Load cookies using regular expressions
        cookies = load_cookies(cookies_file)

        # Select random board and command
        board = random.choice(boards)
        command = random.choice(commands)

        # Prepare payload
        payload = {
            'esp_id': board['esp_id'],
            'command': command
        }

        # Send POST request
        response = requests.post(f"{server_url}/command", cookies=cookies, data=payload)

        # Check response
        if response.status_code == 200:
            print(f"Command '{command}' sent to board '{board['esp_id']}' successfully.")
            return board['esp_id'], command  # Return board ID and sent command
        else:
            print(f"Failed to send command '{command}' to board '{board['esp_id']}'")
            print(f"Status code: {response.status_code}")
            return None, None

    except Exception as e:
        print(f"Error occurred: {e}")
        return None, None

# Function to fetch command from API for a specific board
def fetch_command_from_api(server_url, cookies_file, esp_id):
    try:
        # Load cookies using regular expressions
        cookies = load_cookies(cookies_file)

        # Endpoint for fetching command
        endpoint = f"{server_url}/get_command"

        # Prepare params
        params = {
            'esp_id': esp_id
        }

        # Send GET request
        response = requests.get(endpoint, cookies=cookies, params=params)

        # Check response
        if response.status_code == 200:
            command_data = response.json()
            fetched_command = command_data.get('command', '')
            return fetched_command
        else:
            print(f"Failed to fetch command for board '{esp_id}'")
            print(f"Status code: {response.status_code}")
            return None

    except Exception as e:
        print(f"Error occurred: {e}")
        return None

# Main function to continuously send random commands and verify retrieval
def main():
    boards = load_registered_boards('registered_boards.json')

    if not boards:
        print("No registered boards found. Exiting.")
        return

    command_verification_attempts = 3  # Number of times to verify each command

    while True:
        esp_id, sent_command = send_random_command(server_url, cookies_file, boards)
        
        if esp_id and sent_command:
            verification_success = False
            for _ in range(command_verification_attempts):
                time.sleep(random.uniform(1, 10))  # Random sleep interval between 1 to 10 seconds
                fetched_command = fetch_command_from_api(server_url, cookies_file, esp_id)
                
                if fetched_command == sent_command:
                    print(f"Command '{sent_command}' sent to board '{esp_id}' matches fetched command '{fetched_command}'. Verification successful.")
                    verification_success = True
                    break
                else:
                    print(f"Command '{sent_command}' sent to board '{esp_id}' does not match fetched command '{fetched_command}'. Retrying verification.")

            if not verification_success:
                print(f"Failed to verify command '{sent_command}' for board '{esp_id}' after {command_verification_attempts} attempts.")

        else:
            print("Failed to send command. Skipping verification.")
            time.sleep(random.uniform(1, 10))  # Random sleep interval between 1 to 10 seconds

if __name__ == "__main__":
    main()
