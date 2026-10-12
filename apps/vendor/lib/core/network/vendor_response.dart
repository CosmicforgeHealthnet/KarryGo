import '../contracts/vendor_contract.dart';

class VendorResponse {
  const VendorResponse({
    required this.statusCode,
    required this.headers,
    required this.data,
    this.envelope,
  });
  final int statusCode;
  final Map<String, String> headers;
  final Object? data;
  final JsonObject? envelope;
  T decode<T>(T Function(JsonObject) fromJson) => fromJson(object);
  String? get etag => headers['etag'];
  bool get accepted => statusCode == 202;
  JsonObject get object => jsonObject(data);
  List<Object?> get list {
    if (data is! List) throw const FormatException('Expected a JSON list');
    return List<Object?>.from(data as List);
  }
}
