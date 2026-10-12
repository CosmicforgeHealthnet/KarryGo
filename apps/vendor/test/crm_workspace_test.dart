import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:vendor/features/crm/state/crm_workspace_controller.dart';
import 'package:vendor/core/contracts/vendor_contract.dart';
import 'package:vendor/core/network/vendor_command.dart';
import 'finance_test.dart' show FinanceTransport, financeApp;
import 'order_support_test.dart' show SupportRuntime;
class CrmTransport extends FinanceTransport {
 bool lose=false;String suffix='/tasks';
 @override Future<http.StreamedResponse> send(http.BaseRequest request)async{
  final response=await super.send(request);
  if(lose&&request.method=='POST'&&request.url.path.endsWith(suffix)){lose=false;await response.stream.drain<void>();throw http.ClientException('Accepted CRM response lost');}
  return response;
 }
}
void main(){
 test('CRM activity, atomic membership and profile CAS recovery',()async{
  final transport=CrmTransport(),app=await financeApp(transport),runtime=SupportRuntime(app);
  final customers=CrmWorkspaceController(runtime,'$crmBase/customers');await customers.load();expect(customers.error,isNull);
  final profile=jsonObject(customers.rows.first['profile']);final profileId=profile['profile_ref'] as String;
  final tasks=CrmWorkspaceController(runtime,'$crmBase/tasks');await tasks.load();transport.lose=true;
  expect(await tasks.save(method:'POST',route:'$crmBase/tasks',body:{'customer_profile_ref':profileId,'title':'Check the delivery','description':'Confirm receipt','priority':'NORMAL','visibility':'BUSINESS','assigned_staff_ref':'mock-staff-1'}),isNull);
  final key=tasks.pending!['key'];tasks.dispose();
  final restored=CrmWorkspaceController(runtime,'$crmBase/tasks');await restored.load();expect(restored.rows.length,1);expect(restored.pending!['key'],key);
  final task=await restored.save();expect(task,isNotNull,reason:restored.error);await restored.load();expect(restored.rows.length,1);
  final taskParams=<String,String>{'taskId':task!['task_ref']};
  final started=await restored.save(method:'POST',route:'$crmBase/tasks/{taskId}/start',body:{'expected_version':1},params:taskParams);expect(started?['status'],'IN_PROGRESS');
  final finished=await restored.save(method:'POST',route:'$crmBase/tasks/{taskId}/complete',body:{'expected_version':2},params:taskParams);expect(finished?['status'],'COMPLETED');
  final history=(await app.crmApi.request('GET','$crmBase/tasks/{taskId}/history',pathParameters:taskParams)).object['items'] as List;expect(history.length,3);
  final interactions=CrmWorkspaceController(runtime,'$crmBase/interactions');await interactions.load();
  final interaction=await interactions.save(method:'POST',route:'$crmBase/interactions',body:{'customer_profile_ref':profileId,'type':'NOTE','direction':'INTERNAL','outcome':'NONE','subject':'Customer called','note':'Delivery query','visibility':'BUSINESS','occurred_at':'2026-10-10T08:00:00Z'});expect(interaction,isNotNull,reason:interactions.error);
  expect(await interactions.save(method:'POST',route:'$crmBase/interactions/{interactionId}/correct',params:{'interactionId':interaction!['interaction_ref']},body:{'expected_version':1,'note':'Delivery was confirmed','outcome':'RESOLVED','correction_reason':'Updated after confirmation'}),isNotNull,reason:interactions.error);
  final revisions=(await app.crmApi.request('GET','$crmBase/interactions/{interactionId}/revisions',pathParameters:{'interactionId':interaction['interaction_ref']})).object['items'] as List;expect(revisions.length,2);expect(revisions.first['note'],'Delivery query');
  final tags=CrmWorkspaceController(runtime,'$crmBase/tags');await tags.load();final tag=await tags.save(method:'POST',route:'$crmBase/tags',body:{'name':'Regulars','scope':{'type':'BUSINESS'}});expect(tag,isNotNull,reason:tags.error);
  final bulk=await tags.save(method:'POST',route:'$crmBase/tags/bulk-assign',body:{'tag_ref':tag!['tag_ref'],'profile_refs':[profileId]});expect(bulk?['applied'],1);
  final duplicate=await tags.save(method:'POST',route:'$crmBase/tags/bulk-assign',body:{'tag_ref':tag['tag_ref'],'profile_refs':[profileId]});expect(duplicate?['applied'],0);expect(duplicate?['skipped'],1);
  final segments=CrmWorkspaceController(runtime,'$crmBase/segments');await segments.load();final segment=await segments.save(method:'POST',route:'$crmBase/segments',body:{'name':'Known visitors','segment_type':'STATIC','scope':{'type':'BUSINESS'}});expect(segment,isNotNull,reason:segments.error);
  final bad=await segments.save(method:'POST',route:'$crmBase/segments/{segmentId}/members/bulk-add',params:{'segmentId':segment!['segment_ref']},body:{'profile_refs':[profileId,'not-a-customer']});expect(bad,isNull);expect(segments.pending,isNull);
  final members=(await app.crmApi.request('GET','$crmBase/segments/{segmentId}/members',pathParameters:{'segmentId':segment['segment_ref']})).object['items'] as List;expect(members,isEmpty);
  final relationship=CrmWorkspaceController(runtime,'$crmBase/customers/{profileId}',parameters:{'profileId':profileId});await relationship.load();transport.suffix='/archive';transport.lose=true;
  expect(await relationship.save(method:'POST',route:'$crmBase/customers/{profileId}/archive',body:{},params:{'profileId':profileId},headerVersion:profile['version']),isNull);expect(relationship.pending,isNotNull);relationship.dispose();
  final reopened=CrmWorkspaceController(runtime,'$crmBase/customers/{profileId}',parameters:{'profileId':profileId});await reopened.load();expect(reopened.pending,isNull);expect(reopened.record!['profile']['status'],'ARCHIVED');expect(reopened.record!['profile']['version'],2);
  for(final c in [customers,restored,interactions,tags,segments,reopened])c.dispose();app.client.close();
 });
 test('loyalty ladder and corporate references follow exact backend contracts',()async{
  final app=await financeApp(FinanceTransport()),runtime=SupportRuntime(app);
  final program=CrmWorkspaceController(runtime,'$crmBase/loyalty/program');await program.load();expect(program.record!['version'],0);
  expect(await program.save(method:'PATCH',route:'$crmBase/loyalty/program',body:{'expected_version':0,'status':'ACTIVE','manual_adjustment_limit':1000}),isNotNull,reason:program.error);
  final tiers=CrmWorkspaceController(runtime,'$crmBase/loyalty/tiers');await tiers.load();
  expect(await tiers.save(method:'PUT',route:'$crmBase/loyalty/tiers',body:{'tiers':[{'tier_key':'GOLD','display_name':'Gold','minimum_lifetime_points':500},{'tier_key':'SILVER','display_name':'Silver','minimum_lifetime_points':100}]}),isNotNull,reason:tiers.error);await tiers.load();expect(tiers.record!['tiers'].first['tier_key'],'SILVER');
  final customers=CrmWorkspaceController(runtime,'$crmBase/customers');await customers.load();final profileId=customers.rows.first['profile']['profile_ref'] as String;
  final adjustment=await customers.save(method:'POST',route:'$crmBase/customers/{profileId}/loyalty/adjustments',body:{'direction':'CREDIT','points':120,'reason':'Service recovery'},params:{'profileId':profileId});expect(adjustment?['summary']['available_points'],120,reason:customers.error);
  final reversed=await customers.save(method:'POST',route:'$crmBase/loyalty/entries/{entryId}/reverse',params:{'entryId':adjustment!['entry']['entry_ref']},body:{'reason':'Wrong adjustment'});expect(reversed?['summary']['available_points'],0);
  final ledger=(await app.crmApi.request('GET','$crmBase/customers/{profileId}/loyalty/entries',pathParameters:{'profileId':profileId})).object['entries'] as List;expect(ledger.length,2);expect(ledger.last['reversed_by_entry_ref'],isNotNull);
  final corp=CrmWorkspaceController(runtime,'$crmBase/corporate-customers');await corp.load();
  final account=await corp.save(method:'POST',route:'$crmBase/corporate-customers',body:{'organization_name':'Office team','organization_type':'COMPANY','scope_mode':'BUSINESS','account_manager_staff_ref':'mock-staff-1'});expect(account,isNotNull,reason:corp.error);
  final params=<String,String>{'corporateId':account!['corporate_ref']};
  final contact=await corp.save(method:'POST',route:'$crmBase/corporate-customers/{corporateId}/contacts',params:params,body:{'customer_profile_ref':profileId,'relationship_role':'PRIMARY_CONTACT','is_primary_contact':true,'status':'ACTIVE'});expect(contact,isNotNull,reason:corp.error);
  final location=await corp.save(method:'POST',route:'$crmBase/corporate-customers/{corporateId}/locations',params:params,body:{'location_code':'HQ','name':'Head office','location_type':'HEADQUARTERS','status':'ACTIVE'});expect(location,isNotNull,reason:corp.error);
  final membership=await corp.save(method:'POST',route:'$crmBase/corporate-customers/{corporateId}/memberships',params:params,body:{'contact_ref':contact!['contact_ref'],'location_ref':location!['location_ref'],'membership_role':'MEMBER','status':'ACTIVE'});expect(membership,isNotNull,reason:corp.error);
  final cross=await corp.save(method:'POST',route:'$crmBase/corporate-customers/{corporateId}/memberships',params:params,body:{'contact_ref':contact['contact_ref'],'location_ref':'another-account-location','membership_role':'MEMBER'});expect(cross,isNull);expect(corp.pending,isNull);
  final count=(await app.crmApi.request('GET','$crmBase/corporate-customers/{corporateId}',pathParameters:params)).object['summary']['membership_count'];expect(count,1);
  final registry=(await app.crmApi.request('GET','$crmBase/segment-fields')).object;expect(registry['schema_version'],1);
  final preview=(await app.crmApi.request('POST','$crmBase/segments/preview',body:VendorPayload({'scope':{'type':'BUSINESS'},'rule':{'schema_version':registry['schema_version'],'root':{'field':'profile.subject_type','operator':'EQ','value':'CUSTOMER'}},'sample_limit':10}),command:VendorCommand.create())).object;expect(preview['match_count'],1);
  for(final c in [program,tiers,customers,corp])c.dispose();app.client.close();
 });
}
