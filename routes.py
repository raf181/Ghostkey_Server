from flask import request, jsonify
from flask_sqlalchemy import SQLAlchemy

def register_routes(app, db):
    class Command(db.Model):
        id = db.Column(db.Integer, primary_key=True)
        esp_id = db.Column(db.String(80), nullable=False)
        command = db.Column(db.String(120), nullable=False)

    @app.route('/register', methods=['POST'])
    def register():
        esp_id = request.json.get('esp_id')
        # Registration logic can be expanded if needed
        return jsonify({'message': 'ESP32 registered successfully', 'esp_id': esp_id})

    @app.route('/command', methods=['POST', 'GET'])
    def command():
        if request.method == 'POST':
            esp_id = request.json.get('esp_id')
            command_text = request.json.get('command')
            new_command = Command(esp_id=esp_id, command=command_text)
            db.session.add(new_command)
            db.session.commit()
            return jsonify({'message': 'Command added successfully'})

        elif request.method == 'GET':
            esp_id = request.args.get('esp_id')
            command = Command.query.filter_by(esp_id=esp_id).order_by(Command.id).first()
            if command:
                db.session.delete(command)
                db.session.commit()
                return jsonify({'command': command.command})
            return jsonify({'message': 'No commands'}), 204
