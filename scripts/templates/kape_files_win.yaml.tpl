name: Windows.KapeFiles.Targets
description: |

    Kape is a popular bulk collector tool for triaging a system
    quickly. While KAPE itself is not an opensource tool, the logic it
    uses to decide which files to collect is encoded in YAML files
    hosted on the KapeFiles project
    (https://github.com/EricZimmerman/KapeFiles) and released under an
    MIT license.

    This artifact is automatically generated from these YAML files,
    contributed and maintained by the community. This artifact only
    encapsulates the KAPE "Targets" - basically a bunch of glob
    expressions used for collecting files on the endpoint. We do not
    do any post processing of these files - we just collect them.

    We recommend that timeouts and upload limits be used
    conservatively with this artifact because we can upload really
    vast quantities of data very quickly.

    NOTE: This artifact was built from The KapeFile Repository
    commit %(kape_latest_hash)s dated %(kape_latest_date)s.

reference:
  - https://www.kroll.com/en/insights/publications/cyber/kroll-artifact-parser-extractor-kape
  - https://github.com/EricZimmerman/KapeFiles

parameters:
  - name: UseAutoAccessor
    description: |
      Uses file accessor when possible instead of ntfs parser - this
      is much faster. Note that when using VSS analysis we have to use
      the ntfs accessor for everything which will be much slower.
    type: bool
    default: Y

  - name: Device
    description: |
      Name of the drive letter to search. You can add multiple drives
      separated with a comma.
    default: "C:,D:"

  - name: VSSAnalysisAge
    type: int
    default: 0
    description: |
      If larger than zero we analyze VSS within this many days
      ago. (e.g 7 will analyze all VSS within the last week).  Note
      that when using VSS analysis we have to use the ntfs accessor
      for everything which will be much slower.

%(parameters)s
  - name: KapeRules
    type: hidden
    description: A CSV file controlling the different Kape Target Rules
    default: |
%(csv)s
  - name: KapeTargets
    type: hidden
    description: |
      Each parameter above represents a group of rules to be
      triggered. This table specifies which rule IDs will be included
      when the parameter is checked.
    default: |
%(rules)s

  - name: NTFS_CACHE_TIME
    type: int
    description: How often to flush the NTFS cache. (Default is never).
    default: "1000000"

sources:
  - name: All File Metadata
    query: |
      LET VSS_MAX_AGE_DAYS <= VSSAnalysisAge

      -- Filter the KapeTargets list by the groups that are enabled in
      -- the scope. Only the rows which contain a Group name defined
      -- as TRUE in the scope (parameter) will be included. We then
      -- merge all the Ids into a single flattened list we can check
      -- against.
      LET targets <= SELECT * FROM foreach(row={
        SELECT * FROM parse_csv(accessor="data", filename=KapeTargets)
        WHERE get(member=Group) AND log(message="Selecting " + Group)
      }, query={
        SELECT _value AS Id FROM foreach(row=RuleIds)
      })

      LET EnabledIds <= targets.Id

      -- Filter only the rules in the rule table that have an Id we
      -- want. Targets with $ in their name probably refer to ntfs
      -- special files and so they are designated as ntfs
      -- accessor. Other targets may need ntfs parsing but not
      -- necessary - they are designated with the lazy_ntfs accessor.
      LET rule_specs_ntfs <= SELECT Id, Glob
        FROM parse_csv(filename=KapeRules, accessor="data")
        WHERE Id in EnabledIds AND Accessor='ntfs'
        AND log(message="ntfs: Selecting glob " + Glob)

      LET rule_specs_lazy_ntfs <= SELECT Id, Glob
        FROM parse_csv(filename=KapeRules, accessor="data")
        WHERE Id in EnabledIds AND Accessor='lazy_ntfs'
        AND log(message="auto: Selecting glob " + Glob)

      -- Call the generic VSS file collector with the globs we want in
      -- a new CSV file.
      LET all_results_from_device(Device) = SELECT * FROM if(
           condition=VSSAnalysisAge > 0,
           then={
              -- Process everything with the ntfs_vss accessor.
              SELECT * FROM Artifact.Generic.Collectors.File(
                Root=Device,
                Accessor="ntfs_vss",
                collectionSpec=rule_specs_ntfs + rule_specs_lazy_ntfs)
           }, else={
             SELECT * FROM chain(async=TRUE,
               a={

                   -- Special files we access with the ntfs parser.
                   SELECT * FROM Artifact.Generic.Collectors.File(
                      Root=Device,
                      Accessor="ntfs",
                      collectionSpec=rule_specs_ntfs)
               }, b={

                   -- Prefer the auto accessor if possible since it
                   -- will fall back to ntfs if required but otherwise
                   -- will be faster.
                   SELECT * FROM Artifact.Generic.Collectors.File(
                      Root=Device,
                      Accessor=if(condition=UseAutoAccessor,
                                  then="auto", else="lazy_ntfs"),
                      collectionSpec=rule_specs_lazy_ntfs)
               })
           })

      // This materializes all the files into memory and then into
      // a tempfile if the list is too long.
      LET all_results <= SELECT * FROM foreach(
        row=split(string=Device, sep="\\s*,\\s*"),
        query={
          SELECT * FROM all_results_from_device(Device=_value)
        })

      SELECT * FROM all_results WHERE _Source =~ "Metadata"

  - name: Uploads
    query: |
      SELECT * FROM all_results WHERE _Source =~ "Uploads"

    notebook:
    - type: vql_suggestion
      name: Post process collection
      template: |
        /*

        # Post process this collection.

        Uncomment the following and evaluate the cell to create new
        collections based on the files collected from this artifact.

        The below VQL will apply remapping so standard artifacts will
        see the KapeFiles.Targets collection below as a virtual
        Windows Client. The artifacts will be collected to a temporary
        container and then re-imported as new collections into this
        client.

        NOTE: This is only a stop gap in case the proper artifacts
        were not collected in the first place. Parsing artifacts
        through a remapped collection is not as accurate as parsing
        directly on the endpoint. See
        https://docs.velociraptor.app/training/playbooks/preservation/
        for more info.

        */
        LET _ <= import(artifact="Windows.KapeFiles.Remapping")

        LET tmp <= tempfile()

        LET Results = SELECT import_collection(filename=Container, client_id=ClientId) AS Import
        FROM collect(artifacts=[
                       "Windows.Forensics.Usn",
                       "Windows.NTFS.MFT",
                     ],
                     args=dict(`Windows.Forensics.Usn`=dict(),
                               `Windows.NTFS.MFT`=dict()),
                     output=tmp,
                     remapping=GetRemapping(FlowId=FlowId, ClientId=ClientId))

        // SELECT * FROM Results
