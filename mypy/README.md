## fastapi sample
- start service  
uvicorn main:app --host 0.0.0.0 --port 80 --reload


- upload file  
curl -XPUT --data-binary @main.py localhost/file/m.py

