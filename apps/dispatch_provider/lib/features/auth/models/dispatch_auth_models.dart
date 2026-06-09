class AuthStartResponse {
  final String message;
  final int expiresInSeconds;

  AuthStartResponse({
    required this.message,
    required this.expiresInSeconds,
  });

  factory AuthStartResponse.fromJson(Map<String, dynamic> json) {
    return AuthStartResponse(
      message: json['message'] as String? ?? '',
      expiresInSeconds: (json['expires_in_seconds'] as num?)?.toInt() ?? 0,
    );
  }
}

class AuthVerifyResponse {
  final String providerId;
  final String role;
  final String tokenType;
  final String accessToken;
  final String refreshToken;
  final int expiresInSeconds;

  AuthVerifyResponse({
    required this.providerId,
    required this.role,
    required this.tokenType,
    required this.accessToken,
    required this.refreshToken,
    required this.expiresInSeconds,
  });

  factory AuthVerifyResponse.fromJson(Map<String, dynamic> json) {
    return AuthVerifyResponse(
      providerId: json['provider_id'] as String? ?? '',
      role: json['role'] as String? ?? '',
      tokenType: json['token_type'] as String? ?? 'Bearer',
      accessToken: json['access_token'] as String? ?? '',
      refreshToken: json['refresh_token'] as String? ?? '',
      expiresInSeconds: (json['expires_in_seconds'] as num?)?.toInt() ?? 0,
    );
  }
}

class AuthRefreshResponse {
  final String accessToken;
  final String refreshToken;
  final String tokenType;
  final int expiresInSeconds;

  AuthRefreshResponse({
    required this.accessToken,
    required this.refreshToken,
    required this.tokenType,
    required this.expiresInSeconds,
  });

  factory AuthRefreshResponse.fromJson(Map<String, dynamic> json) {
    return AuthRefreshResponse(
      accessToken: json['access_token'] as String? ?? '',
      refreshToken: json['refresh_token'] as String? ?? '',
      tokenType: json['token_type'] as String? ?? 'Bearer',
      expiresInSeconds: (json['expires_in_seconds'] as num?)?.toInt() ?? 0,
    );
  }
}
