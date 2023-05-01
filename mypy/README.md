## 0. Install dependence
pip3 install -r requirements.txt


##  Start sample
- start fastapi_async service  
uvicorn --host 0.0.0.0 --port 5000 --reload fastapi_async:app

- start fastapi_sync service
uvicorn --host 0.0.0.0 --port 5000 --reload fastapi_sync:app

- start flask service

- upload file  
curl -X PUT --data-binary @main.py localhost:5000/file/m.py

- put item
curl -X PUT -d '{"name":"name1", "level":2, "desc":"desc2"}' -H'content-type:application/json' localhost:5000/items/1 

