# routes.py
from flask import Blueprint, request, jsonify, render_template, redirect, url_for, flash
from models import db, User, Device, Log, CommandQueue
from forms import RegistrationForm, LoginForm
from datetime import datetime
from werkzeug.security import generate_password_hash, check_password_hash
from flask_login import login_user, current_user, logout_user, login_required
import os

# Create a blueprint instance
blueprint_name = Blueprint('blueprint_name', __name__)

# Load the registration key from environment variables or config
registration_key = os.getenv('REGISTRATION_KEY', 'default_key')

def commit_to_both_databases():
    db.session.commit()
    db.get_engine(bind='backup').execute("COMMIT")

@blueprint_name.route('/register', methods=['GET', 'POST'])
def register():
    form = RegistrationForm()
    if form.validate_on_submit():
        username = form.username.data
        password = form.password.data
        email = form.email.data or None  # Handle optional email

        hashed_password = generate_password_hash(password, method='pbkdf2:sha256')

        new_user = User(username=username, password=hashed_password, email=email)
        db.session.add(new_user)
        db.session.commit()

        return jsonify({"message": "User registered successfully"}), 201

    return render_template('register.html', form=form)

@blueprint_name.route('/login', methods=['GET', 'POST'])
def login():
    form = LoginForm()
    if form.validate_on_submit():
        username = form.username.data
        password = form.password.data
        user = User.query.filter_by(username=username).first()

        if user and check_password_hash(user.password, password):
            login_user(user)
            return redirect(url_for('dashboard'))  # Redirect to the dashboard or any other page
        else:
            return jsonify({"message": "Invalid username or password"}), 401

    return render_template('login.html', form=form)

@blueprint_name.route('/logout')
@login_required
def logout():
    logout_user()
    return redirect(url_for('login'))

@blueprint_name.route('/register_board', methods=['POST'])
@login_required
def register_board():
    data = request.json
    board_id = data.get('board_id')
    user_id = data.get('user_id')

    user = User.query.get(user_id)
    if not user:
        return jsonify({"error": "User not found"}), 404

    new_board = Device(device_id=board_id, user_id=user_id, last_seen=datetime.utcnow())
    db.session.add(new_board)
    commit_to_both_databases()

    return jsonify({"message": "Board registered successfully"}), 201

@blueprint_name.route('/connect_log', methods=['POST'])
@login_required
def log_connection():
    data = request.json
    user_id = data.get('user_id')
    action = data.get('action')

    if not user_id or not action:
        return jsonify({"error": "user_id and action are required"}), 400

    new_log = Log(user_id=user_id, action=action, timestamp=datetime.utcnow())
    db.session.add(new_log)
    commit_to_both_databases()

    return jsonify({"message": "Connection logged successfully"}), 200

@blueprint_name.route('/')
def home():
    return render_template('status.html')

@blueprint_name.route('/command', methods=['POST'])
@login_required
def command():
    data = request.json
    device_id = data.get('device_id')

    if not device_id:
        return jsonify({"error": "device_id is required"}), 400

    log = Log(device_id=device_id, action="check-in", timestamp=datetime.utcnow())
    db.session.add(log)
    commit_to_both_databases()

    device = Device.query.filter_by(device_id=device_id).first()
    if device:
        device.last_seen = datetime.utcnow()
    else:
        user = data.get('user', 'unknown')
        device = Device(device_id=device_id, user=user, last_seen=datetime.utcnow())
        db.session.add(device)
    commit_to_both_databases()

    pending_commands = CommandQueue.query.filter_by(device_id=device_id, executed=False).all()
    commands = [cmd.command for cmd in pending_commands]

    for cmd in pending_commands:
        cmd.executed = True
    commit_to_both_databases()

    return jsonify({"commands": commands})

@blueprint_name.route('/send_command', methods=['POST'])
@login_required
def send_command():
    data = request.json
    device_id = data.get('device_id')
    command = data.get('command')

    if not device_id or not command:
        return jsonify({"error": "device_id and command are required"}), 400

    command_entry = CommandQueue(device_id=device_id, command=command)
    db.session.add(command_entry)
    commit_to_both_databases()

    return jsonify({"message": "Command added to the queue"}), 200

@blueprint_name.route('/status', methods=['GET'])
@login_required
def status():
    devices = Device.query.all()
    response = [{"device_id": device.device_id, "last_seen": device.last_seen, "user": device.user} for device in devices]
    return jsonify(response)