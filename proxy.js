import { next } from '@vercel/functions';
import { NO_STORE, prepareUpstreamHeaders } from './deploy/vercel/proxy-security.js';
export default function proxy(request) {
 const headers=prepareUpstreamHeaders(request);
 if(headers instanceof Response)return headers;
 return next({request:{headers},headers:NO_STORE});
}
