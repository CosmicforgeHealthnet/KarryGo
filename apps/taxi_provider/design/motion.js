/* Local vector state illustrations. No downloads, timers, or camera access. */
function stateIllustration(kind, compact = false) {
  const car = `<g class="motion-car"><ellipse cx="155" cy="139" rx="64" ry="7" class="motion-shadow"/><path d="M100 102l14-26h60l24 26 17 8v23H90v-24Z" class="motion-car-body"/><path d="m119 82-10 20h34V82Zm32 0v20h36l-19-20Z" class="motion-window"/><path d="M97 110h14m90 0h9M151 111h10" class="motion-detail"/><circle cx="116" cy="131" r="13" class="motion-wheel"/><circle cx="189" cy="131" r="13" class="motion-wheel"/><circle cx="116" cy="131" r="5" class="motion-hub"/><circle cx="189" cy="131" r="5" class="motion-hub"/></g>`;
  const pin = `<g class="motion-pin"><path d="M245 49c0 15-17 30-17 30s-17-15-17-30a17 17 0 1 1 34 0Z" fill="#00c2a8"/><circle cx="228" cy="49" r="6" fill="#0b1f44"/></g>`;
  const town = `<g class="motion-town"><path d="M30 110V62h29v48m9 0V42h34v68m102 0V68h25v42m14 0V55h28v55"/><path d="M39 72h11m-11 12h11m27-30h14m-14 13h14m122 12h8m30-12h10"/></g>`;
  let scene;
  if (kind === 'search') scene = `<circle cx="150" cy="90" r="63" class="motion-radar"/><circle cx="150" cy="90" r="43" class="motion-radar second"/><path d="M44 125h32V55h94v69h91" class="motion-route"/>${pin}${car}<circle cx="62" cy="49" r="5" class="motion-beacon"/>`;
  else if (kind === 'verify') scene = `<g class="motion-phone"><rect x="111" y="22" width="78" height="132" rx="15" class="motion-car-body"/><rect x="119" y="35" width="62" height="101" rx="5" class="motion-paper"/><path d="M140 29h20" class="motion-detail"/><g class="motion-qr"><path d="M130 53h15v15h-15Zm27 0h15v15h-15Zm-27 28h15v15h-15Zm27 0h7v7h-7Zm9 9h7v8h-7Z"/></g><path d="M123 49h54" class="motion-scan"/></g><path d="m65 96 8 8 14-17" class="motion-check small"/><circle cx="228" cy="68" r="22" class="motion-radar"/>`;
  else if (kind === 'success') scene = `<circle cx="150" cy="88" r="63" class="motion-radar"/><circle cx="150" cy="88" r="45" fill="#00c2a8"/><path d="m129 88 14 14 29-32" class="motion-check"/><g class="motion-confetti"><path d="m62 49 8 7m167 49 9 2M84 133l-4 9m133-98 7-9"/><circle cx="63" cy="106" r="4"/><circle cx="237" cy="61" r="4"/></g>`;
  else scene = `${town}<path d="M20 146h260" class="motion-ground"/><g class="motion-road"><path d="M30 157h35m30 0h35m30 0h35m30 0h35"/></g>${car}${kind === 'drive' ? pin : '<g class="motion-sun"><circle cx="235" cy="40" r="14"/><path d="M235 17v-5m0 51v5m23-28h5m-51 0h-5"/></g>'}`;
  return `<div class="state-illustration motion-${kind}${compact ? ' compact' : ''}" aria-hidden="true"><svg viewBox="0 0 300 180" fill="none" xmlns="http://www.w3.org/2000/svg">${scene}</svg></div>`;
}

// Schematic preview, not geographic data or a navigable map.
function mapPreview(searching = false) {
  return `<div class="trip-map ${searching ? 'search-map' : ''}" role="img" aria-label="Illustrative map showing a driver near pickup and a sample route to drop-off; not live navigation">
    <svg viewBox="0 0 350 240" xmlns="http://www.w3.org/2000/svg" aria-hidden="true">
      <rect width="350" height="240" class="map-land"/>
      <path d="M284 0h66v240H230l20-37 17-31-5-35 22-38Z" class="map-water"/>
      <g class="map-block"><path d="M14 15h61v40H14Zm77 0h74v40H91Zm-77 60h61v53H14Zm77 0h74v53H91Zm91-60h57v40h-57ZM14 149h61v66H14Zm77 0h74v66H91Zm91 0h40v43h-40Z"/></g>
      <path d="M182 75h48v49h-48Z" class="map-park"/>
      <g class="map-street"><path d="M0 65h278M0 139h260M83 0v240M174 0v240M0 225h238M245 0l-6 86 12 55-28 99"/></g>
      <g class="map-street-center"><path d="M0 65h278M0 139h260M83 0v240M174 0v240M0 225h238"/></g>
      <g class="map-label"><text x="18" y="39">LEKKI</text><text x="97" y="110">Oniru</text><text x="184" y="103">Park</text><text x="273" y="191" transform="rotate(-65 273 191)">Lagos Lagoon</text><text x="18" y="186">VICTORIA</text><text x="18" y="199">ISLAND</text></g>
      <path d="M83 102v37h91v53h49" class="map-route-halo"/>
      <path d="M83 102v37h91v53h49" class="map-route"/>
      ${searching ? '<circle cx="83" cy="102" r="32" class="map-search-ring"/><circle cx="83" cy="102" r="52" class="map-search-ring outer"/>' : ''}
      <g transform="translate(83 102)"><circle r="13" fill="#fff"/><circle r="9" fill="#00c2a8"/><path d="m0-6 5 10-5-2-5 2Z" fill="#0b1f44"/></g>
      <g transform="translate(223 192)"><path d="M0 0s-13-15-13-24a13 13 0 1 1 26 0C13-15 0 0 0 0" fill="#0b1f44" stroke="#fff" stroke-width="2"/><circle cy="-24" r="4" fill="#00c2a8"/></g>
      <g class="map-place"><rect x="102" y="79" width="109" height="25" rx="8"/><text x="112" y="95">The Palms · Pickup</text></g>
    </svg><span class="map-caption">Illustrative map · Sample route</span><span class="map-distance">${searching ? 'Finding nearby trips' : '8.4 km · 24 min'}</span></div>`;
}
