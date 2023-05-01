#!/usr/bin/env python3

from typing import Union
import json
import os
from fastapi import FastAPI, Request
from fastapi.responses import FileResponse, JSONResponse
from pydantic import BaseModel
import uvicorn

store = os.getcwd()

app = FastAPI()


@app.put("/file/{filename}")
async def put_file(filename: str, req: Request):
    with open(os.path.join(store, filename), "wb") as fd:
        content = await req.body()
        fd.write(content)
    return JSONResponse(content={"filename": filename}, status_code=200)


@app.get('/file')
async def list_file():
    files = os.listdir(store)
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

class Item(BaseModel):
    name: str
    level: int
    #timestamp: datetime
    desc: Union[str, None] = None

def load_items():
    items = {}
    try:
        with open(os.path.join(store, 'items.json'), "r") as fd:
            items = json.load(fd)
    finally:
        return items

@app.get('/items/{id}')
async def get_items(id: int):
    items = load_items()
    return JSONResponse(content={"code":0, 'data':items.get(id, {})}, status_code=200)
        
    
@app.put('/items/{id}')
async def put_items(id: int, item: Item):
    items = load_items()
    items[id] = item.__dict__
    with open(os.path.join(store, 'items.json'), "w+") as fd:
        json.dump(items, fd)
    return JSONResponse(content={"id": id, 'name': item.name}, status_code=200)

if __name__ == "__main__":
    uvicorn.run("main:app", host="0.0.0.0", port=5000)
