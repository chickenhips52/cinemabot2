
container_build:
	docker build -t sdcinemabot .

container_run:
	docker run -p 8000:8000 sdcinemabot

