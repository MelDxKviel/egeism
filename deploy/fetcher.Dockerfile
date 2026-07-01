# Python content-fetcher service (wraps РЕШУ/sdamgia). Build context = repo root.
FROM python:3.12-slim
# git is needed to install the sdamgia fork from source (see requirements.txt).
RUN apt-get update && apt-get install -y --no-install-recommends git \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY tools/fetch/ ./
# Core scraper deps (openfipi информатика source) MUST install — they carry the
# real ФИПИ-with-answers fetch, so they can't depend on the flaky sdamgia git
# install below.
RUN pip install --no-cache-dir requests beautifulsoup4
# The sdamgia fork (РЕШУ, for rus/math/soc) is best-effort: if it fails (no
# internet at build, repo moved, etc.) the service still runs — информатика uses
# openfipi, and rus/math/soc simply return empty (no demo fallback) — so a flaky
# install must not break the image build.
RUN pip install --no-cache-dir -r requirements.txt || echo "warn: sdamgia install failed; rus/math/soc fetch will return empty"
EXPOSE 8090
CMD ["python", "server.py"]
