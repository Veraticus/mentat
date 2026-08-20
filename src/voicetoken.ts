import { createHmac, randomUUID } from 'node:crypto';

interface TokenGrant {
  token: string;
  room: string;
  url: string;
  expires_at: string;
}

export interface TokenIssuer {
  issue(): TokenGrant;
}

interface TokenIssuerConfig {
  apiKey: string;
  apiSecret: string;
  url: string;
}

interface TokenIssuerDeps {
  now?: () => Date;
  uuid?: () => string;
  sign?: (input: string, secret: string) => string;
}

function hmacSha256(input: string, secret: string): string {
  return createHmac('sha256', secret).update(input).digest('base64url');
}

export function createTokenIssuer(
  config: TokenIssuerConfig,
  deps: TokenIssuerDeps = {},
): TokenIssuer {
  const now = deps.now ?? (() => new Date());
  const uuid = deps.uuid ?? randomUUID;
  const sign = deps.sign ?? hmacSha256;

  return {
    issue(): TokenGrant {
      const id = uuid();
      const room = `android-${id}`;
      const issuedAt = Math.floor(now().getTime() / 1000);
      const expiresAt = issuedAt + 3600;
      const header = Buffer.from(JSON.stringify({ alg: 'HS256', typ: 'JWT' })).toString(
        'base64url',
      );
      const payload = Buffer.from(
        JSON.stringify({
          iss: config.apiKey,
          sub: `pixel-${id}`,
          iat: issuedAt,
          nbf: issuedAt,
          exp: expiresAt,
          video: {
            room,
            roomJoin: true,
            canPublish: true,
            canPublishSources: ['microphone'],
            canSubscribe: true,
            canPublishData: true,
          },
        }),
      ).toString('base64url');
      const signingInput = `${header}.${payload}`;
      const signature = sign(signingInput, config.apiSecret);

      return {
        token: `${signingInput}.${signature}`,
        room,
        url: config.url,
        expires_at: new Date(expiresAt * 1000).toISOString(),
      };
    },
  };
}
