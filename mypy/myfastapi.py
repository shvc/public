#!/usr/bin/env python3

import os
from fastapi import FastAPI, Request
from fastapi.responses import FileResponse, JSONResponse
import uvicorn
import shutil

store = os.getcwd()

app = FastAPI()


@app.put("/file/{filename}")
async def put_file(filename: str, req: Request):
    with open(os.path.join(store, filename), "wb") as bw:
        content = await req.body()
        await bw.write(content)
    return JSONResponse(content={"filename": filename}, status_code=200)


@app.get('/file')
async def list_file():
    files = await os.listdir(store)
    return JSONResponse(content={"file": files}, status_code=200)


@app.get('/file/{filename}')
async def get_file(filename: str):
    return FileResponse(path=os.path.join(store, filename), filename=filename)


@app.delete('/file/{filename}')
async def delete_file(filename: str):
    try:
        os.remove(os.path.join(store, filename))
        return JSONResponse(content={
            "removed": True,
        }, status_code=200)
    except Exception as e:
        return JSONResponse(content={
            "removed": True,
            "error": str(e),
        }, status_code=500)

if __name__ == "__main__":
    uvicorn.run("main:app", host="0.0.0.0", port=80)
