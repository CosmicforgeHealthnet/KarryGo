library;

class ApiCoreConfig {
  const ApiCoreConfig({required this.baseUrl});

  final String baseUrl;

  Uri uri(String path, [Map<String, String>? queryParameters]) {
    final normalizedBase = baseUrl.endsWith('/')
        ? baseUrl.substring(0, baseUrl.length - 1)
        : baseUrl;
    final normalizedPath = path.startsWith('/') ? path : '/$path';

    return Uri.parse(
      '$normalizedBase$normalizedPath',
    ).replace(queryParameters: queryParameters);
  }
}

class ApiErrorCode {
  static const badRequest = 'bad_request';
  static const validationFailed = 'validation_failed';
  static const unauthorized = 'unauthorized';
  static const forbidden = 'forbidden';
  static const notFound = 'not_found';
  static const conflict = 'conflict';
  static const rateLimited = 'rate_limited';
  static const serviceUnavailable = 'service_unavailable';
  static const internalError = 'internal_error';
  static const network = 'network_error';
  static const unknown = 'unknown_error';

  const ApiErrorCode._();
}

class ApiFieldError {
  const ApiFieldError({required this.field, required this.message});

  final String field;
  final String message;

  factory ApiFieldError.fromJson(Map<String, dynamic> json) {
    return ApiFieldError(
      field: json['field']?.toString() ?? '',
      message: json['message']?.toString() ?? 'Invalid value.',
    );
  }
}

class ApiProblem {
  const ApiProblem({
    required this.code,
    required this.message,
    this.requestId,
    this.fields = const [],
    this.details = const {},
  });

  final String code;
  final String message;
  final String? requestId;
  final List<ApiFieldError> fields;
  final Map<String, dynamic> details;

  factory ApiProblem.fromJson(Map<String, dynamic> json) {
    final rawFields = json['fields'];
    final rawDetails = json['details'];

    return ApiProblem(
      code: json['code']?.toString() ?? ApiErrorCode.unknown,
      message:
          json['message']?.toString() ??
          'Something went wrong. Please try again.',
      requestId: json['request_id']?.toString(),
      fields: rawFields is List
          ? rawFields
                .whereType<Map>()
                .map(
                  (field) =>
                      ApiFieldError.fromJson(Map<String, dynamic>.from(field)),
                )
                .toList()
          : const [],
      details: rawDetails is Map
          ? Map<String, dynamic>.from(rawDetails)
          : const {},
    );
  }
}

class ApiException implements Exception {
  const ApiException({
    required this.code,
    required this.message,
    this.statusCode,
    this.requestId,
    this.fields = const [],
    this.details = const {},
    this.cause,
  });

  final String code;
  final String message;
  final int? statusCode;
  final String? requestId;
  final List<ApiFieldError> fields;
  final Map<String, dynamic> details;
  final Object? cause;

  bool get isAuthFailure =>
      statusCode == 401 || code == ApiErrorCode.unauthorized;

  bool get isRetryable =>
      code == ApiErrorCode.network ||
      code == ApiErrorCode.serviceUnavailable ||
      statusCode == 408 ||
      statusCode == 429 ||
      (statusCode != null && statusCode! >= 500);

  bool get hasFieldErrors => fields.isNotEmpty;

  String get title {
    return switch (code) {
      ApiErrorCode.validationFailed => 'Check your details',
      ApiErrorCode.unauthorized => 'Session expired',
      ApiErrorCode.forbidden => 'Access denied',
      ApiErrorCode.notFound => 'Not found',
      ApiErrorCode.rateLimited => 'Too many attempts',
      ApiErrorCode.network => 'Connection problem',
      ApiErrorCode.serviceUnavailable => 'Service unavailable',
      _ => 'Something went wrong',
    };
  }

  factory ApiException.fromProblem(
    ApiProblem problem, {
    int? statusCode,
    Object? cause,
  }) {
    return ApiException(
      code: problem.code,
      message: problem.message,
      statusCode: statusCode,
      requestId: problem.requestId,
      fields: problem.fields,
      details: problem.details,
      cause: cause,
    );
  }

  factory ApiException.fromErrorEnvelope(
    Map<String, dynamic> json, {
    int? statusCode,
    Object? cause,
  }) {
    final rawError = json['error'];
    if (rawError is Map) {
      return ApiException.fromProblem(
        ApiProblem.fromJson(Map<String, dynamic>.from(rawError)),
        statusCode: statusCode,
        cause: cause,
      );
    }

    return ApiException(
      code: ApiErrorCode.unknown,
      message: 'Something went wrong. Please try again.',
      statusCode: statusCode,
      cause: cause,
    );
  }

  factory ApiException.network(Object cause) {
    return ApiException(
      code: ApiErrorCode.network,
      message: 'Something went wrong. Check your connection and try again.',
      cause: cause,
    );
  }

  @override
  String toString() {
    final status = statusCode == null ? '' : '($statusCode)';
    return 'ApiException$status [$code]: $message';
  }
}

sealed class ApiResult<T> {
  const ApiResult();

  bool get isSuccess => this is ApiSuccess<T>;
  bool get isFailure => this is ApiFailure<T>;

  R when<R>({
    required R Function(T data) success,
    required R Function(ApiException error) failure,
  }) {
    final self = this;
    if (self is ApiSuccess<T>) {
      return success(self.data);
    }

    return failure((self as ApiFailure<T>).error);
  }
}

final class ApiSuccess<T> extends ApiResult<T> {
  const ApiSuccess(this.data);

  final T data;
}

final class ApiFailure<T> extends ApiResult<T> {
  const ApiFailure(this.error);

  final ApiException error;
}

abstract interface class TokenStore {
  Future<String?> readAccessToken();
  Future<void> saveAccessToken(String token);
  Future<void> clear();
}

class InMemoryTokenStore implements TokenStore {
  String? _accessToken;

  @override
  Future<String?> readAccessToken() async => _accessToken;

  @override
  Future<void> saveAccessToken(String token) async {
    _accessToken = token;
  }

  @override
  Future<void> clear() async {
    _accessToken = null;
  }
}
