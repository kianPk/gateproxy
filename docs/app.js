const $ = (id) => document.getElementById(id)
let token = localStorage.getItem('gateproxy_token') || ''

async function refreshHealth() {
  try {
    const h = await fetch('/healthz').then((r) => r.json())
    $('health').textContent = h.ok ? 'ONLINE' : 'DOWN'
    $('health').classList.toggle('ok', !!h.ok)
  } catch {
    $('health').textContent = 'OFFLINE'
  }
  try {
    const [raw, status] = await Promise.all([
      fetch('/metrics').then((r) => r.text()),
      fetch('/v1/status').then((r) => r.json()).catch(() => ({})),
    ])
    const get = (k) => {
      const m = raw.match(new RegExp(`^${k} ([0-9.]+)$`, 'm'))
      return m ? m[1] : '0'
    }
    const open = status.breaker && status.breaker.open
      ? Object.keys(status.breaker.open).length
      : 0
    $('kpis').innerHTML = [
      ['Requests', get('gateproxy_requests_total')],
      ['Proxied', get('gateproxy_proxied_total')],
      ['Rate limited', get('gateproxy_rate_limited_total')],
      ['Circuits open', String(open)],
    ]
      .map(([l, v]) => `<div><b>${v}</b><span>${l}</span></div>`)
      .join('')
  } catch {
    $('kpis').innerHTML = '<div><b>—</b><span>metrics</span></div>'
  }
}

async function loadRoutes() {
  const data = await fetch('/v1/routes').then((r) => r.json())
  $('routes').innerHTML = (data.routes || [])
    .map((r) => {
      const roles = (r.roles || []).join(',') || '—'
      const auth = r.auth ? `JWT (${roles})` : 'public'
      return `<div class="route"><code>${r.prefix}</code><span class="badge">${auth}</span></div>`
    })
    .join('')
}

$('mint').onclick = async () => {
  const res = await fetch('/v1/token', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      sub: $('sub').value,
      admin_token: $('admin').value,
      role: $('role').value,
    }),
  })
  const data = await res.json().catch(() => ({}))
  if (!res.ok) {
    $('tokenOut').textContent = data.message || res.statusText || 'failed'
    return
  }
  token = data.token
  localStorage.setItem('gateproxy_token', token)
  $('tokenOut').textContent = `${data.role || 'user'}\n${token}`
}

$('probe').onclick = async () => {
  const path = $('path').value || '/httpbin/get'
  const headers = {}
  if (token) headers.Authorization = 'Bearer ' + token
  const t0 = performance.now()
  try {
    const res = await fetch(path, { headers })
    const text = await res.text()
    const rid = res.headers.get('X-Request-Id') || '—'
    $('probeOut').textContent =
      `${res.status} in ${Math.round(performance.now() - t0)}ms\nid=${rid}\n\n` + text.slice(0, 2000)
  } catch (e) {
    $('probeOut').textContent = String(e)
  }
  refreshHealth()
}

if (token) $('tokenOut').textContent = token
refreshHealth()
loadRoutes()
setInterval(refreshHealth, 4000)
