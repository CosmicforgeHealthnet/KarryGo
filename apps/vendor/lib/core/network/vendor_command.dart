import 'dart:math';

/// Preserve this object on identical retries. Version belongs to If-Match only
/// when the operation declares that header; body versions live in its DTO.
class VendorCommand {
  const VendorCommand({this.idempotencyKey, this.ifMatch});
  final String? idempotencyKey;
  final String? ifMatch;
  factory VendorCommand.create({int? version}) => VendorCommand(
    idempotencyKey: List.generate(
      24,
      (_) => Random.secure().nextInt(256).toRadixString(16).padLeft(2, '0'),
    ).join(),
    ifMatch: version?.toString(),
  );
}
