from fastapi import FastAPI
import uvicorn

app = FastAPI(title="Pumpkin Quant Service")

@app.get("/api/health")
def health_check():
    return {"status": "online", "service": "Pumpkin Quant Engine"}

@app.post("/api/evaluate")
def evaluate_strategy(data: dict):
    # TODO: This will receive K-line data from Go, run Pandas strategy, and return signals
    return {"signal": "BUY", "confidence": 0.95}

if __name__ == "__main__":
    uvicorn.run(app, host="0.0.0.0", port=8000)
