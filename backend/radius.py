"""Mock RADIUS implementation for authentication and accounting.

This provides simulated RADIUS authentication and accounting capabilities
for development and testing purposes. Real RADIUS server integration
is deferred per user request.

The module implements the RADIUS protocol attributes and methods
needed for captive portal authentication, session management,
and accounting tracking. All operations are mocked and can be
extended with real RADIUS server connectivity later.
"""

import hashlib
import time
from typing import Optional, Dict, Any, List
from dataclasses import dataclass


@dataclass
class RadiusAttribute:
    code: int
    value: Any


def make_radius_packet(identifier: int = 0, code: int = 1, attributes: Optional[List[RadiusAttribute]] = None) -> dict:
    """Create a RADIUS packet for authentication or accounting."""
    attrs = attributes or []
    attr_dict = {}
    for attr in attrs:
        attr_dict[attr.code] = attr.value
    return {
        'identifier': identifier,
        'code': code,
        'attributes': attr_dict
    }


def authenticate_user(username: str, password: str, radius_secret: str = 'mock-secret') -> dict:
    """Simulate RADIUS authentication of a user.
    
    In a real implementation, this would send a RADIUS Access-Request
    to a RADIUS server and receive an Access-Accept/Reject.
    """
    # Simulate authentication check
    if not username or not password:
        return {'status': 'reject', 'reason': 'Missing credentials'}
    
    # Mock: accept any valid-looking credentials
    if len(username) >= 3 and len(password) >= 1:
        # Generate a simulated session token
        token = hashlib.sha256((username + password).encode()).hexdigest()[:32]
        return {
            'status': 'accept',
            'session_token': token,
            'user': username,
            'attributes': {
                ' frameless_ip': '10.0.0.1',
                'frameless_framed_prefix': '10.0.0.0/24',
                'frameless_routing': 'subnet',
            }
        }
    
    return {'status': 'reject', 'reason': 'Authentication failed'}


def accounting_start(session_token: str, user: str, nas_ip: str = '10.0.0.1') -> dict:
    """Simulate RADIUS accounting start event."""
    return {
        'event': 'accounting_start',
        'session_token': session_token,
        'user': user,
        'nas_ip': nas_ip,
        'timestamp': int(time.time()),
        'status': 'recorded'
    }


def accounting_stop(session_token: str, user: str, nas_ip: str = '10.0.0.1', duration: int = 0) -> dict:
    """Simulate RADIUS accounting stop event."""
    return {
        'event': 'accounting_stop',
        'session_token': session_token,
        'user': user,
        'nas_ip': nas_ip,
        'duration': duration,
        'timestamp': int(time.time()),
        'status': 'recorded'
    }
