// GENERATED from actual request schemas/Go JSON fields.
import '../../../core/contracts/vendor_contract.dart';

class PostCommerceOrdersVendorOrderIdSupportCasesRequest extends VendorPayload {
  PostCommerceOrdersVendorOrderIdSupportCasesRequest({
    String? category,
    String? description,
    String? vendorOrderRef,
    List<dynamic>? lineRefs,
  }) : super({
         'category': ?category,
         'description': ?description,
         'vendor_order_ref': ?vendorOrderRef,
         'line_refs': ?lineRefs,
       });
  PostCommerceOrdersVendorOrderIdSupportCasesRequest.fromJson(super.json);
  String? get category => value('category') as String?;
  String? get description => value('description') as String?;
  String? get vendorOrderRef => value('vendor_order_ref') as String?;
  List<dynamic>? get lineRefs => value('line_refs') as List<dynamic>?;
}
