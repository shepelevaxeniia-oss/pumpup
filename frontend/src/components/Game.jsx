import React, {useState, useEffect} from 'react'

const BASE = import.meta.env.VITE_API_BASE || 'http://localhost:8080'

function headers(userId){
  return {'Content-Type':'application/json','X-User-Id': userId}
}

export default function Game(){
  const [userId, setUserId] = useState(localStorage.getItem('pumpup_user') || '')
  const [balance, setBalance] = useState(0)
  const [round, setRound] = useState(null)
  const [difficulty, setDifficulty] = useState('easy')
  const [log, setLog] = useState([])

  useEffect(()=> {
    if(userId) fetchBalance()
  }, [userId])

  async function newUser(){
    const username = 'user-' + Math.random().toString(36).slice(2,7)
    const res = await fetch(BASE + '/auth/login', {
      method:'POST', headers:{'Content-Type':'application/json'}, body: JSON.stringify({username})
    })
    const j = await res.json()
    localStorage.setItem('pumpup_user', j.user_id)
    setUserId(j.user_id)
  }

  async function fetchBalance(){
    const res = await fetch(BASE + '/balance', {headers: headers(userId)})
    const j = await res.json()
    setBalance(j.balance)
  }

  async function startRound(){
    const stake = 1000
    const clientSeed = 'client-' + Math.random().toString(36).slice(2,8)
    const res = await fetch(BASE + '/rounds/start', {
      method:'POST', headers: headers(userId),
      body: JSON.stringify({stake, client_seed: clientSeed, difficulty})
    })
    const j = await res.json()
    setRound(j)
    setLog(l => [`started ${j.round_id}`, ...l])
    fetchBalance()
  }

  async function doStep(){
    if(!round) return
    const res = await fetch(BASE + '/rounds/step', {
      method:'POST', headers: headers(userId),
      body: JSON.stringify({round_id: round.round_id, difficulty})
    })
    const j = await res.json()
    setLog(l => [JSON.stringify(j), ...l])
    if(j.result === 'exploded') {
      setRound(null)
      fetchBalance()
    } else {
      // update local round display
      setRound(r => ({...r, step: j.step, multiplier: j.multiplier}))
    }
  }

  async function cashout(){
    if(!round) return
    const res = await fetch(BASE + '/rounds/cashout', {
      method:'POST', headers: headers(userId),
      body: JSON.stringify({round_id: round.round_id})
    })
    const j = await res.json()
    setLog(l => [JSON.stringify(j), ...l])
    setRound(null)
    fetchBalance()
  }

  return (
    <div>
      {!userId ? <button onClick={newUser}>Create demo user</button> : <div>
        <div>user: {userId}</div>
        <div>Balance: {balance}</div>
        <div>
          Difficulty:
          <select value={difficulty} onChange={e=>setDifficulty(e.target.value)}>
            <option value="easy">Легкий</option>
            <option value="medium">Средний</option>
            <option value="hard">Сложный</option>
          </select>
        </div>
        {!round ? <button onClick={startRound}>Начать (1000)</button> : <>
          <div style={{marginTop:10}}>
            <div>Round: {round.round_id}</div>
            <div>Step: {round.step}</div>
            <div>Multiplier: {round.multiplier}</div>
            <button onClick={doStep}>Шаг</button>
            <button onClick={cashout}>Забрать выигрыш</button>
          </div>
        </>}
        <div style={{marginTop:10}}>
          <h4>Logs</h4>
          <div style={{maxHeight:200, overflow:'auto', background:'#f3f3f3', padding:8}}>
            {log.map((l,i)=><div key={i}><small>{l}</small></div>)}
          </div>
        </div>
      </div>}
    </div>
  )
}
