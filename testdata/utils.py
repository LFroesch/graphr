def format_response(data):
    return str(data)

def validate(payload):
    result = format_response(payload)
    return len(result) > 0

class Processor:
    def run(self):
        validate(self.data)
