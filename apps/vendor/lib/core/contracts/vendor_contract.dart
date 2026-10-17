import 'dart:convert';

typedef JsonObject = Map<String, dynamic>;

JsonObject jsonObject(Object? value) {
  if (value is! Map) throw const FormatException('Expected a JSON object');
  return Map<String, dynamic>.from(value);
}

class VendorPayload {
  VendorPayload(JsonObject json)
    : _json = jsonObject(jsonDecode(jsonEncode(json)));
  final JsonObject _json;
  JsonObject toJson() => jsonObject(jsonDecode(jsonEncode(_json)));
  Object? value(String key) => _json[key];
}

/// Exact contract metadata, not a client authorization or transition engine.
class VendorOperation {
  VendorOperation.fromJson(JsonObject json)
    : id = json['id'] as String,
      method = json['method'] as String,
      path = json['path'] as String,
      source = json['source'] as String,
      authenticated = json['authenticated'] as bool,
      parameters = (json['parameters'] as List).map(jsonObject).toList(),
      requestSchema = json['request_schema'] == null
          ? null
          : jsonObject(json['request_schema']),
      requestKnown = json['request_known'] as bool,
      requestRequired = json['request_required'] as bool,
      permission = json['permission'] as String?;
  final String id, method, path, source;
  final bool authenticated, requestKnown, requestRequired;
  final List<JsonObject> parameters;
  final JsonObject? requestSchema;
  final String? permission;
  bool get isRead => method == 'GET' || method == 'HEAD';
  Iterable<JsonObject> parametersIn(String location) =>
      parameters.where((p) => p['in'] == location);
  bool requiresHeader(String name) => parametersIn('header').any(
    (p) =>
        (p['name'] as String).toLowerCase() == name.toLowerCase() &&
        p['required'] == true,
  );
}

/// Local contract errors are separate from backend ApiException responses.
class VendorContractException implements Exception {
  const VendorContractException(this.message);
  final String message;
  @override
  // Shown when a screen prints the error; the technical detail stays in
  // [message] for logs and tests.
  String toString() =>
      'This request could not be sent. Check the details and try again.';
}

/// Drops `null` values for optional properties the backend does not declare
/// nullable. Go decodes a missing optional field and a null the same way, so
/// omitting it keeps the request valid instead of rejecting it on the phone.
Object? pruneOptionalNulls(Object? value, JsonObject schema) {
  if (value is Map && schema['properties'] is Map) {
    final props = jsonObject(schema['properties']);
    final required = (schema['required'] as List?) ?? const [];
    final out = <String, dynamic>{};
    for (final e in value.entries) {
      final key = e.key as String;
      final prop = props[key] is Map ? jsonObject(props[key]) : null;
      if (e.value == null &&
          !required.contains(key) &&
          prop != null &&
          prop['nullable'] != true &&
          !(prop['type'] is List && (prop['type'] as List).contains('null'))) {
        continue;
      }
      out[key] = prop == null ? e.value : pruneOptionalNulls(e.value, prop);
    }
    return out;
  }
  if (value is List && schema['items'] is Map) {
    final item = jsonObject(schema['items']);
    return [for (final v in value) pruneOptionalNulls(v, item)];
  }
  return value;
}

void validateRequest(Object? value, JsonObject schema, [String at = 'body']) {
  if (value == null) {
    if (schema['nullable'] == true ||
        schema.isEmpty ||
        (schema['type'] is List && (schema['type'] as List).contains('null'))) {
      return;
    }
    throw VendorContractException('$at cannot be null');
  }
  final types = schema['type'];
  final type = types is List
      ? types.where((e) => e != 'null').firstOrNull
      : types;

  if (schema['enum'] is List && !(schema['enum'] as List).contains(value)) {
    throw VendorContractException('$at is outside the backend enum');
  }
  if (type == 'object' || schema.containsKey('properties')) {
    if (value is! Map) throw VendorContractException('$at must be an object');
    final props = schema['properties'] is Map
        ? jsonObject(schema['properties'])
        : <String, dynamic>{};
    final required = (schema['required'] as List?) ?? const [];
    for (final key in required) {
      if (!value.containsKey(key)) {
        throw VendorContractException('$at.$key is required');
      }
    }
    for (final key in value.keys) {
      if (props.containsKey(key)) {
        validateRequest(value[key], jsonObject(props[key]), '$at.$key');
      } else if (schema['additionalProperties'] == false ||
          (props.isNotEmpty && schema['additionalProperties'] == null)) {
        throw VendorContractException(
          '$at.$key is not a backend request field',
        );
      } else if (schema['additionalProperties'] is Map) {
        validateRequest(
          value[key],
          jsonObject(schema['additionalProperties']),
          '$at.$key',
        );
      }
    }
  } else if (type == 'array') {
    if (value is! List) throw VendorContractException('$at must be an array');
    if (schema['items'] is Map) {
      for (var i = 0; i < value.length; i++) {
        validateRequest(value[i], jsonObject(schema['items']), '$at[$i]');
      }
    }
  } else if (type == 'integer' && value is! int) {
    throw VendorContractException('$at must be an integer');
  } else if (type == 'number' && value is! num) {
    throw VendorContractException('$at must be a number');
  } else if (type == 'string' && value is! String) {
    throw VendorContractException('$at must be a string');
  } else if (type == 'boolean' && value is! bool) {
    throw VendorContractException('$at must be a boolean');
  }
  if (value is num) {
    if (schema['minimum'] is num && value < (schema['minimum'] as num)) {
      throw VendorContractException('$at below minimum');
    }
    if (schema['maximum'] is num && value > (schema['maximum'] as num)) {
      throw VendorContractException('$at above maximum');
    }
  }
  if (value is String) {
    if (schema['minLength'] is int &&
        value.length < (schema['minLength'] as int)) {
      throw VendorContractException('$at too short');
    }
    if (schema['maxLength'] is int &&
        value.length > (schema['maxLength'] as int)) {
      throw VendorContractException('$at too long');
    }
  }
  if (schema['allOf'] is List) {
    for (final part in schema['allOf'] as List) {
      validateRequest(value, jsonObject(part), at);
    }
  }
}
