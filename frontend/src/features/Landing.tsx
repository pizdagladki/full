import { Link } from 'react-router-dom';

// Landing — the public face of the game: the cartoon world, the pitch, one CTA.
export function Landing() {
  return (
    <div className="panel-screen entrance" data-testid="landing-screen">
      <div className="entrance-logo">ГЛЯДЕЛКИ</div>
      <div className="entrance-tagline">игра в гляделки на рейтинг, славу и корону</div>
      <ul className="entrance-points">
        <li>👁️ смотри в камеру — моргнул, значит проиграл</li>
        <li>⚡ живые дуэли один на один</li>
        <li>👑 сгони короля с горы</li>
      </ul>
      <Link className="btn-mode entrance-google" to="/">
        Играть
      </Link>
    </div>
  );
}
