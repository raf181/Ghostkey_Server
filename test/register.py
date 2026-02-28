import requests
import json

def register_board(server_url, cookies_file, start_id, end_id, output_file):
    try:
        # Load cookies from cookies.txt file
        with open(cookies_file, 'r') as f:
            cookies = {}
            for line in f:
                parts = line.strip().split('\t')
                if len(parts) >= 2:
                    cookies[parts[0]] = parts[1].strip()

        # Endpoint for registration
        endpoint = f"{server_url}/register_device"

        registered_boards = []

        for esp_id in range(start_id, end_id + 1):
            esp_secret_key = f'esp_secret_key_{esp_id}'  # Generate secret key based on esp_id

            payload = {
                'esp_id': f'esp32_{esp_id}',
                'esp_secret_key': esp_secret_key
            }

            # Send POST request
            response = requests.post(endpoint, cookies=cookies, data=payload)

            # Check response
            if response.status_code == 200:
                response_data = response.json()
                if response_data.get('message') == 'ESP32 registered successfully':
                    print(f"ESP32 with ID 'esp32_{esp_id}' registered successfully with secret key: {esp_secret_key}.")
                    registered_boards.append({
                        'esp_id': f'esp32_{esp_id}',
                        'esp_secret_key': esp_secret_key
                    })
                else:
                    print(f"Registration failed for esp32_{esp_id}. Server response: {response_data}")
            else:
                print(f"Failed to register esp32_{esp_id}. Status code: {response.status_code}")

        # Write registered boards to output file
        with open(output_file, 'w') as outfile:
            json.dump(registered_boards, outfile, indent=4)

    except FileNotFoundError:
        print(f"Error: {cookies_file} not found.")

    except Exception as e:
        print(f"Error occurred: {e}")

# Example usage:
if __name__ == "__main__":
    server_url = 'http://127.0.0.1:5002'
    cookies_file = 'cookies.txt'  # Adjust path if necessary
    start_id = 100001
    end_id = 200000
    output_file = 'registered_boards.json'  # Output file to store registered boards

    register_board(server_url, cookies_file, start_id, end_id, output_file)