export default function Home() {
  return (
    <div style={{ fontFamily: 'system-ui, sans-serif', padding: '40px', maxWidth: '800px', margin: '0 auto' }}>
      <h1 style={{ fontSize: '3rem', fontWeight: 'bold' }}>🎃 Pumpkin Trader Pro</h1>
      <p style={{ fontSize: '1.5rem', color: '#666' }}>Next Generation Quantitative Trading Terminal</p>
      
      <div style={{ marginTop: '40px', padding: '20px', backgroundColor: '#f5f5f5', borderRadius: '8px' }}>
        <h3>🚀 System Status</h3>
        <ul>
          <li>Frontend (React/Next.js): 🟢 Online</li>
          <li>Backend API (Go): ⏳ Connecting...</li>
          <li>Quant Engine (Python): ⏳ Connecting...</li>
        </ul>
      </div>
    </div>
  )
}
