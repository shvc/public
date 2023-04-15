#!/usr/bin/env python3

from os import path
from flask import Flask, request


app = Flask("myapp")

store = '/tmp/'


@app.route('/hello')
def hello():
    return "hello, world"


@app.route('/file/<string:filename>', methods=['GET'])
def get_file(filename):
    print(request.method, 'file', filename)
    print(request.headers)
    return filename


@app.route('/file/<string:filename>', methods=['PUT'])
def put_file(filename):
    print(request.method, 'file', filename)
    print('headers', request.headers)
    content_type = request.headers.get('Content-Type')
    if (content_type == 'application/x-www-form-urlencoded'):
        return 'Content-Type not support!'
    with open(path.join(store, filename), "wb") as buffer:
        buffer.write(request.data)
    return filename + " <-success\n"


if __name__ == '__main__':
    app.run(debug=True)
