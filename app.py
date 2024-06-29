from flask import Flask, request, jsonify
from routes import register_routes
from flask_sqlalchemy import SQLAlchemy
import logging
from logging.handlers import RotatingFileHandler

app = Flask(__name__)

# Configure logging
handler = RotatingFileHandler('app.log', maxBytes=10000, backupCount=1)
handler.setLevel(logging.INFO)
formatter = logging.Formatter('%(asctime)s - %(levelname)s - %(message)s')
handler.setFormatter(formatter)
app.logger.addHandler(handler)

# Debugging statement
app.logger.info('Logging initialized')

# Database configuration
app.config['SQLALCHEMY_DATABASE_URI'] = 'sqlite:///database.db'
app.config['SQLALCHEMY_TRACK_MODIFICATIONS'] = False

db = SQLAlchemy(app)

# Register routes
register_routes(app, db)

# Log requests from ESP32 devices
@app.before_request
def log_request_info():
    if request.path == '/command':
        esp_id = request.json.get('esp_id') if request.json else None
        command = request.json.get('command') if request.json else None
        app.logger.info('ESP32 Request - esp_id: %s, command: %s', esp_id, command)

if __name__ == '__main__':
    with app.app_context():
        db.create_all()  # Create database tables if they do not exist
    app.run(host='0.0.0.0', port=5000)
