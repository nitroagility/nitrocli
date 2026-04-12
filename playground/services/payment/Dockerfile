FROM python:3.13-alpine
ARG BUILD_VERSION=unknown
LABEL version="${BUILD_VERSION}"
WORKDIR /app
COPY requirements.txt ./
RUN pip install --no-cache-dir -r requirements.txt
COPY app.py ./
EXPOSE 8081
CMD ["python", "app.py"]
