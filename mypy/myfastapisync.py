#!/usr/bin/env python3

import os
from fastapi import FastAPI, Request
from fastapi.responses import FileResponse, JSONResponse
import uvicorn
import shutil

store = os.getcwd()

app = FastAPI()


@app.put("/file/{filename}")
def put_file(filename: str, req: Request):
    with open(os.path.join(store, filename), "wb") as buffer:
        content = req.body()
        buffer.write(content)
        buffer.close()
    return JSONResponse(content={"filename": filename}, status_code=200)


@app.get('/file')
def list_file():
    return JSONResponse(content={"file": os.listdir(store)}, status_code=200)


@app.get('/file/{filename}')
def get_file(filename: str):
    return FileResponse(path=os.path.join(store, filename), filename=filename)


@app.delete('/file/{filename}')
def delete_file(filename: str):
    try:
        os.remove(os.path.join(store, filename))
        return JSONResponse(content={
            "removed": True
        }, status_code=200)
    except FileNotFoundError:
        return JSONResponse(content={
            "removed": False,
            "error_message": "File not found"
        }, status_code=404)


if __name__ == "__main__":
    uvicorn.run("main:app", host="0.0.0.0", port=80)
