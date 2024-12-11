import requests
import threading
import random
import string

# Configurable server URL and registration endpoints
SERVER_URL = "http://localhost:5000"
REGISTER_USER_URL = f"{SERVER_URL}/register_user"
REGISTER_DEVICE_URL = f"{SERVER_URL}/register_device"

# Credentials for registration
SECRET_KEY = "your_secret_key"  # Replace with the actual server-side secret key

# User and device counts
USER_COUNT = 1000
DEVICE_COUNT = 1000

# Generate random usernames and passwords
def random_string(length=8):
    return ''.join(random.choice(string.ascii_letters + string.digits) for _ in range(length))

# Register a user
def register_user(user_id):
    username = f"user_{user_id}"
    password = random_string(10)
    data = {
        "username": username,
        "password": password,
        "secret_key": SECRET_KEY
    }
    response = requests.post(REGISTER_USER_URL, data=data)
    if response.status_code == 200:
        print(f"Registered user: {username}")
    else:
        print(f"Failed to register user {username}: {response.status_code}")

# Register a device for a specific user
def register_device(device_id):
    esp_id = f"esp_{device_id}"
    esp_secret_key = random_string(16)
    data = {
        "esp_id": esp_id,
        "esp_secret_key": esp_secret_key,
    }
    response = requests.post(REGISTER_DEVICE_URL, data=data)
    if response.status_code == 200:
        print(f"Registered device: {esp_id}")
    else:
        print(f"Failed to register device {esp_id}: {response.status_code}")

# Run user registrations in parallel
def batch_register_users():
    threads = []
    for user_id in range(1, USER_COUNT + 1):
        t = threading.Thread(target=register_user, args=(user_id,))
        threads.append(t)
        t.start()
        if user_id % 1000 == 0:  # Limit simultaneous threads
            for thread in threads:
                thread.join()
            threads = []

# Run device registrations in parallel
def batch_register_devices():
    threads = []
    for device_id in range(1, DEVICE_COUNT + 1):
        t = threading.Thread(target=register_device, args=(device_id,))
        threads.append(t)
        t.start()
        if device_id % 1000 == 0:  # Limit simultaneous threads
            for thread in threads:
                thread.join()
            threads = []

if __name__ == "__main__":
    print("Starting user registration...")
    batch_register_users()
    print("User registration completed.")

    print("Starting device registration...")
    batch_register_devices()
    print("Device registration completed.")
