FROM python:3.15-alpine

WORKDIR /app

# COPY requirements.txt .
COPY . .

# RUN pip install -r requirements.txt --no-cache-dir

RUN uv sync

# COPY . .

CMD ["uv", "run", "main.py"]
