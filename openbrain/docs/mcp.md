# OpenBrain MCP Interface

This document details the Model Context Protocol (MCP) tools provided by OpenBrain.

## Provided Tools

1. `openbrain_ingest`
   - Description: Ingests memory records into a specific namespace.
   - Parameters:
     - `namespace_id` (string, required): The namespace to ingest into.
     - `records` (array, required): An array of memory objects (id, text, metadata).

2. `openbrain_query`
   - Description: Queries memories within a namespace.
   - Parameters:
     - `namespace_id` (string, required): The namespace to query.
     - `query` (string, required): The text to search for.
     - `limit` (integer, optional): Maximum number of records to return.

3. `openbrain_forget`
   - Description: Forgets specific memory records in a namespace.
   - Parameters:
     - `namespace_id` (string, required): The namespace.
     - `record_ids` (array, required): Array of string IDs to forget.
