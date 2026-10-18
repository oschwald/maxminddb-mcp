package mcp

const batchOutputSchema = `{
 "type":"object", "additionalProperties":false, "required":["results"],
 "properties":{"results":{"type":"array","items":{
  "anyOf":[` + ipOutputSchema + `,{"type":"object","required":["ip","error"],
   "properties":{"ip":{"type":"string"},"error":{"type":"object",
    "required":["code","message"],"properties":{"code":{"type":"string"},"message":{"type":"string"}}}}
  }]
 }}}
}`

// MMDB records have edition-specific fields. Prefixes use their JSON string form.
const networkOutputSchema = `{
 "type":"object", "additionalProperties":false,
 "required":["iterator_id","resume_token","results","has_more","total_processed","total_matched"],
 "properties":{
  "iterator_id":{"type":"string","description":"Identifier for continuing this live query with the same parameters."},
  "resume_token":{"type":"string","description":"Token for resuming after the last returned record if the live iterator expires."},
  "has_more":{"type":"boolean","description":"True when another matching record exists. Continue with the same query parameters."},
  "total_processed":{"type":"integer","minimum":0,"description":"Cumulative records processed across pages, including nonmatching records scanned ahead to find the next match."},
  "total_matched":{"type":"integer","minimum":0,"description":"Cumulative matching records returned across pages, not the count on this page."},
  "results":{"type":"array","items":{
   "type":"object","additionalProperties":false,"required":["network","data"],
   "properties":{"network":{"type":"string"},"data":{}}
  }}
 }
}`

const ipOutputSchema = `{
 "type":"object", "required":["ip"],
 "properties":{"ip":{"type":"string"}},
 "oneOf":[
  {"required":["database","found","network","data"],
   "properties":{
    "database":{"type":"string"},"found":{"type":"boolean"},
    "network":{"type":["string","null"]},"data":{}
   }},
  {"required":["databases"],"properties":{
   "databases":{"type":"object","additionalProperties":{
    "type":"object","required":["found","network","data"],
    "properties":{
     "found":{"type":"boolean"},"network":{"type":["string","null"]},"data":{}
    }
   }}
  }}
 ]
}`
