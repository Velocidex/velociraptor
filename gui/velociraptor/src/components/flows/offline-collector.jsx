import { HotKeys, ObserveKeys } from "react-hotkeys";
import {
    NewCollectionLaunch,
    NewCollectionRequest,
    NewCollectionSelectArtifacts,
    PaginationBuilder
} from './new-collection.jsx';

import Col from 'react-bootstrap/Col';
import Form from 'react-bootstrap/Form';
import Modal from 'react-bootstrap/Modal';
import NewCollectionConfigParameters from './new-collections-parameters.jsx';
import PropTypes from 'prop-types';
import React from 'react';
import Row from 'react-bootstrap/Row';
import StepWizard from 'react-step-wizard';
import T from '../i8n/i8n.jsx';
import ToolViewer from "../tools/tool-viewer.jsx";
import ValidatedInteger from '../forms/validated_int.jsx';
import _ from 'lodash';

// The offline collection wizard is built upon the new collection
// wizard with some extra steps.
class OfflinePaginator extends PaginationBuilder {
    PaginationSteps = ["Select Artifacts", "Configure Parameters",
                       "Configure Collection", "Specify Resources",
                       "Review", "Launch"];
}

const tool_name_lookup = {
    Windows: "VelociraptorWindows",
    Windows_x86: "VelociraptorWindows_x86",
    Linux: "VelociraptorLinux",
    MacOS: "VelociraptorDarwin",
}


class OfflineCollectorParameters  extends React.Component {
    static propTypes = {
        parameters: PropTypes.object,
        setParameters: PropTypes.func.isRequired,
        paginator: PropTypes.object,
    }

    render() {
        return (
            <>
              <Modal.Header closeButton>
                <Modal.Title>{T("Create Offline collector:  Configure Collector")}</Modal.Title>
              </Modal.Header>
              <Modal.Body>
                <Form>
                  <Form.Group as={Row}>
                    <Form.Label column sm="3">{T("Target Operating System")}</Form.Label>
                    <Col sm="8">
                      <Form.Control
                        as="select"
                        value={this.props.parameters.target_os}
                        onChange={(e) => {
                            this.props.parameters.target_os = e.currentTarget.value;
                            this.props.setParameters(this.props.parameters);
                        }}
                      >
                        <option value="Windows">Windows</option>
                        <option value="Windows_x86">Windows_x86</option>
                        <option value="Linux">Linux</option>
                        <option value="MacOS">Mac OS</option>
                      </Form.Control>
                    </Col>
                  </Form.Group>

                  <Form.Group as={Row}>
                    <Form.Label column sm="3">{T("Encryption Scheme")}</Form.Label>
                    <Col sm="8">
                      <Form.Control
                        as="select"
                        value={this.props.parameters.encryption_scheme}
                        onChange={(e) => {
                            this.props.parameters.encryption_scheme = e.currentTarget.value;
                            this.props.setParameters(this.props.parameters);
                        }}
                      >
                        <option value="None">{T("None")}</option>
                        <option value="X509">{T("X509 Certificate/Frontend Cert")}</option>
                        <option value="PGP">{T("PGP Encryption")}</option>
                        <option value="Password">{T("Password")}</option>
                      </Form.Control>
                    </Col>
                  </Form.Group>
                  {(this.props.parameters.encryption_scheme === "PGP" ||
                    this.props.parameters.encryption_scheme === "X509") &&
                   <Form.Group as={Row} >
                     <Form.Label column sm="3">{T("Public Key/Cert")}</Form.Label>
                     <Col sm="8">
                       <Form.Control
                         as="textarea"
                         placeholder={T("Public Key/Certificate To Encrypt With. If X509, Defaults To Frontend Cert")}
                         spellCheck="false"
                         value={this.props.parameters.encryption_args.public_key}
                         onChange={e => {
                             this.props.parameters.encryption_args.public_key = e.target.value;
                             this.props.setParameters(this.props.parameters);
                         }} />
                     </Col>
                   </Form.Group>
                  }
                  { this.props.parameters.encryption_scheme === "Password" &&
                    <Form.Group as={Row} >
                      <Form.Label column sm="3">{T("Password")}</Form.Label>
                      <Col sm="8">
                        <Form.Control
                          as="input"
                          placeholder={T("Password")}
                          spellCheck="false"
                          value={this.props.parameters.encryption_args.password}
                          onChange={e => {
                              this.props.parameters.encryption_args.password = e.target.value;
                              this.props.setParameters(this.props.parameters);
                          }} />
                      </Col>
                    </Form.Group>
                  }

                  <Form.Group as={Row}>
                    <Form.Label column sm="3">{T("Collection Type")}</Form.Label>
                    <Col sm="8">
                      <Form.Control
                        as="select"
                        value={this.props.parameters.target}
                        onChange={(e) => {
                            this.props.parameters.target = e.currentTarget.value;
                            this.props.setParameters(this.props.parameters);
                        }}
                      >
                        <option value="ZIP">{T("Zip Archive")}</option>
                        <option value="GCS">{T("Google Cloud Bucket")}</option>
                        <option value="S3">{T("AWS Bucket")}</option>
                        <option value="Azure">{T("Azure SAS URL")}</option>
                        <option value="SMBShare">{T("SMB Share")}</option>
                        <option value="SFTP">{T("SFTP Upload")}</option>
                      </Form.Control>
                    </Col>
                  </Form.Group>

                  { this.props.parameters.target === "GCS" &&
                    <>
                      <Form.Group as={Row}>
                        <Form.Label column sm="3">{T("GCS Bucket")}</Form.Label>
                        <Col sm="8">
                          <Form.Control
                            as="textarea" rows={3}
                            placeholder={T("Bucket name")}
                            spellCheck="false"
                            value={this.props.parameters.target_args.bucket}
                            onChange={e => {
                                this.props.parameters.target_args.bucket = e.target.value;
                                this.props.setParameters(this.props.parameters);
                            }}
                          />
                        </Col>
                      </Form.Group>

                      <Form.Group as={Row}>
                        <Form.Label column sm="3">{T("GCS Key Blob")}</Form.Label>
                        <Col sm="8">
                          <Form.Control
                            as="textarea" rows={3}
                            placeholder={T("GCS Blob")}
                            spellCheck="false"
                            value={this.props.parameters.target_args.GCSKey}
                            onChange={e => {
                                this.props.parameters.target_args.GCSKey = e.target.value;
                                this.props.setParameters(this.props.parameters);
                            }}
                          />
                        </Col>
                      </Form.Group> </>

                  }
                  { this.props.parameters.target === "SMBShare" &&
                    <>
                      <Form.Group as={Row}>
                        <Form.Label column sm="3">{T("Username")}</Form.Label>
                        <Col sm="8">
                          <Form.Control
                            as="textarea" rows={3}
                            placeholder={T("SMB Share login username")}
                            spellCheck="false"
                            value={this.props.parameters.target_args.username}
                            onChange={e => {
                                this.props.parameters.target_args.username = e.target.value;
                                this.props.setParameters(this.props.parameters);
                            }}
                          />
                        </Col>
                      </Form.Group>
                      <Form.Group as={Row}>
                        <Form.Label column sm="3">{T("Password")}</Form.Label>
                        <Col sm="8">
                          <Form.Control
                            as="textarea" rows={3}
                            placeholder={T("SMB Share login password")}
                            spellCheck="false"
                            value={this.props.parameters.target_args.password}
                            onChange={e => {
                                this.props.parameters.target_args.password = e.target.value;
                                this.props.setParameters(this.props.parameters);
                            }}
                          />
                        </Col>
                      </Form.Group>
                      <Form.Group as={Row}>
                        <Form.Label column sm="3">{T("Server Address")}</Form.Label>
                        <Col sm="8">
                          <Form.Control
                            as="textarea" rows={3}
                            placeholder={T("SMB Share address (e.g. \\\\192.168.1.1:445\\Sharename)")}
                            spellCheck="false"
                            value={this.props.parameters.target_args.server_address}
                            onChange={e => {
                                this.props.parameters.target_args.server_address = e.target.value;
                                this.props.setParameters(this.props.parameters);
                            }}
                          />
                        </Col>
                      </Form.Group>
                    </>
                  }
                  { this.props.parameters.target === "Azure" &&
                    <>
                      <Form.Group as={Row}>
                        <Form.Label column sm="3">{T("Azure SAS URL")}</Form.Label>
                        <Col sm="8">
                          <Form.Control
                            as="textarea" rows={3}
                            placeholder={T("SAS URL as generated from the Azure console")}
                            spellCheck="false"
                            value={this.props.parameters.target_args.sas_url}
                            onChange={e => {
                                this.props.parameters.target_args.sas_url = e.target.value;
                                this.props.setParameters(this.props.parameters);
                            }}
                          />
                        </Col>
                      </Form.Group></>
                  }
                  { this.props.parameters.target === "S3" &&
                    <>
                      <Form.Group as={Row}>
                        <Form.Label column sm="3">{T("S3 Bucket")}</Form.Label>
                        <Col sm="8">
                          <Form.Control
                            as="textarea" rows={3}
                            placeholder={T("Bucket name")}
                            spellCheck="false"
                            value={this.props.parameters.target_args.bucket}
                            onChange={e => {
                                this.props.parameters.target_args.bucket = e.target.value;
                                this.props.setParameters(this.props.parameters);
                            }}
                          />
                        </Col>
                      </Form.Group>

                      <Form.Group as={Row}>
                        <Form.Label column sm="3">{T("Credentials Key")}</Form.Label>
                        <Col sm="8">
                          <Form.Control
                            as="textarea" rows={3}
                            placeholder={T("Credentials Key")}
                            spellCheck="false"
                            value={this.props.parameters.target_args.credentialsKey}
                            onChange={e => {
                                this.props.parameters.target_args.credentialsKey = e.target.value;
                                this.props.setParameters(this.props.parameters);
                            }}
                          />
                        </Col>
                      </Form.Group>

                      <Form.Group as={Row}>
                        <Form.Label column sm="3">{T("Credentials Secret")}</Form.Label>
                        <Col sm="8">
                          <Form.Control
                            as="textarea" rows={3}
                            placeholder={T("Credentials Secret")}
                            spellCheck="false"
                            value={this.props.parameters.target_args.credentialsSecret}
                            onChange={e => {
                                this.props.parameters.target_args.credentialsSecret = e.target.value;
                                this.props.setParameters(this.props.parameters);
                            }}
                          />
                        </Col>
                      </Form.Group>
                      <Form.Group as={Row}>
                        <Form.Label column sm="3">{T("Region")}</Form.Label>
                        <Col sm="8">
                          <Form.Control
                            as="textarea" rows={3}
                            placeholder={T("Region")}
                            spellCheck="false"
                            value={this.props.parameters.target_args.region}
                            onChange={e => {
                                this.props.parameters.target_args.region = e.target.value;
                                this.props.setParameters(this.props.parameters);
                            }}
                          />
                        </Col>
                      </Form.Group>
                      <Form.Group as={Row}>
                        <Form.Label column sm="3">{T("Endpoint")}</Form.Label>
                        <Col sm="8">
                          <Form.Control
                            as="textarea" rows={3}
                            placeholder={T("Endpoint (blank for AWS)")}
                            spellCheck="false"
                            value={this.props.parameters.target_args.endpoint}
                            onChange={e => {
                                this.props.parameters.target_args.endpoint = e.target.value;
                                this.props.setParameters(this.props.parameters);
                            }}
                          />
                        </Col>
                      </Form.Group>
                      <Form.Group as={Row}>
                      <Form.Label column sm="3">File Name Prefix</Form.Label>
                      <Col sm="8">
                        <Form.Control as="textarea" rows={3}
                                        placeholder={T("Prefix for files being uploaded. end in / for folders (blank if not used)")}
                                        spellCheck="false"
                                        value={this.props.parameters.target_args.s3UploadRoot}
                                        onChange={(e) => {
                                            this.props.parameters.target_args.s3UploadRoot = e.target.value;
                                            this.props.setParameters(this.props.parameters);
                                        }}
                        >
                        </Form.Control>
                      </Col>
                    </Form.Group>
                    <Form.Group as={Row}>
                        <Form.Label column sm="3">{T("Server Side Encryption")}</Form.Label>
                        <Col sm="8">
                          <Form.Control
                            as="select"
                            value={this.props.parameters.target_args.serverSideEncryption}
                            onChange={(e) => {
                                this.props.parameters.target_args.serverSideEncryption = e.target.value;
                                this.props.setParameters(this.props.parameters);
                            }}
                          >
                            <option value="">None</option>
                            <option value="aws:kms">aws:kms</option>
                            <option value="AES256">AES256</option>
                          </Form.Control>
                        </Col>
                      </Form.Group>
                      <Form.Group as={Row}>
                      <Form.Label column sm="3">KMS Encryption Key</Form.Label>
                      <Col sm="8">
                        <Form.Control as="textarea" rows={3}
                                        placeholder={T("KMS Encryption Key ARN (blank if KMS not used)")}
                                        spellCheck="false"
                                        value={this.props.parameters.target_args.kmsEncryptionKey}
                                        onChange={(e) => {
                                            this.props.parameters.target_args.kmsEncryptionKey = e.target.value;
                                            this.props.setParameters(this.props.parameters);
                                        }}
                        >
                        </Form.Control>
                      </Col>
                    </Form.Group>
                    <Form.Group as={Row}>
                        <Form.Label column sm="3">{T("Skip Cert Verification")}</Form.Label>
                        <Col sm="8">
                          <Form.Control
                            as="select"
                            value={this.props.parameters.target_args.noverifycert}
                            onChange={(e) => {
                                this.props.parameters.target_args.noverifycert = e.target.value;
                                this.props.setParameters(this.props.parameters);
                            }}
                          >
                            <option value="N">N</option>
                            <option value="Y">Y</option>
                          </Form.Control>
                        </Col>
                      </Form.Group>
                    </>
                  }

                  { this.props.parameters.target === "SFTP" &&
                    <>
                      <Form.Group as={Row}>
                        <Form.Label column sm="3">{T("Upload Path")}</Form.Label>
                        <Col sm="8">
                          <Form.Control
                            as="textarea" rows={3}
                            placeholder={T("Upload Path")}
                            spellCheck="false"
                            value={this.props.parameters.target_args.path}
                            onChange={e => {
                                this.props.parameters.target_args.path = e.target.value;
                                this.props.setParameters(this.props.parameters);
                            }}
                          />
                        </Col>
                      </Form.Group>

                      <Form.Group as={Row}>
                        <Form.Label column sm="3">{T("Private Key")}</Form.Label>
                        <Col sm="8">
                          <Form.Control
                            as="textarea" rows={3}
                            placeholder={T("Private Key")}
                            spellCheck="false"
                            value={this.props.parameters.target_args.privatekey}
                            onChange={e => {
                                this.props.parameters.target_args.privatekey = e.target.value;
                                this.props.setParameters(this.props.parameters);
                            }}
                          />
                        </Col>
                      </Form.Group>

                      <Form.Group as={Row}>
                        <Form.Label column sm="3">{T("User")}</Form.Label>
                        <Col sm="8">
                          <Form.Control
                            as="textarea" rows={3}
                            placeholder={T("User")}
                            spellCheck="false"
                            value={this.props.parameters.target_args.user}
                            onChange={e => {
                                this.props.parameters.target_args.user = e.target.value;
                                this.props.setParameters(this.props.parameters);
                            }}
                          />
                        </Col>
                      </Form.Group>

                      <Form.Group as={Row}>
                        <Form.Label column sm="3">{T("Endpoint")}</Form.Label>
                        <Col sm="8">
                          <Form.Control
                            as="textarea" rows={3}
                            placeholder={T("Endpoint")}
                            spellCheck="false"
                            value={this.props.parameters.target_args.endpoint}
                            onChange={e => {
                                this.props.parameters.target_args.endpoint = e.target.value;
                                this.props.setParameters(this.props.parameters);
                            }}
                          />
                        </Col>
                      </Form.Group>

                      <Form.Group as={Row}>
                        <Form.Label column sm="3">{T("Host Key")}</Form.Label>
                        <Col sm="8">
                          <Form.Control
                            as="textarea" rows={3}
                            placeholder={T("Leave Blank to disable host key checking")}
                            spellCheck="false"
                            value={this.props.parameters.target_args.hostkey}
                            onChange={(e) => {
                                this.props.parameters.target_args.hostkey = e.target.value;
                                this.props.setParameters(this.props.parameters);
                            }}
                          />
                        </Col>
                      </Form.Group>
                    </>
                  }

                  <Form.Group as={Row}>
                    <Form.Label column sm="3">{T("Velociraptor Binary")}</Form.Label>
                    <Col sm="8">
                      <ToolViewer name={tool_name_lookup[this.props.parameters.target_os]}/>
                    </Col>
                  </Form.Group>
                  <Form.Group as={Row}>
                    <Form.Label column sm="3">{T("Temp directory")}</Form.Label>
                    <Col sm="8">
                      <Form.Control
                        as="input"
                        placeholder={T("Temp location")}
                        spellCheck="false"
                        value={this.props.parameters.opt_tempdir}
                        onChange={e => {
                            this.props.parameters.opt_tempdir = e.target.value;
                            this.props.setParameters(this.props.parameters);
                        }}
                      />
                    </Col>
                  </Form.Group>
                  <Form.Group as={Row}>
                    <Form.Label column sm="3">{T("Compression Level")}</Form.Label>
                    <Col sm="8">
                      <ValidatedInteger setValue={value=>{
                          this.props.parameters.opt_level = value;
                          this.props.setParameters(this.props.parameters);
                      }}
                                        value={this.props.parameters.opt_level}
                                        placeholder="Compression Level (0 - 9)"
                      />
                    </Col>
                  </Form.Group>

                  <Form.Group as={Row}>
                    <Form.Label column sm="3">{T("Output format")}</Form.Label>
                    <Col sm="8">
                      <Form.Control
                        as="select"
                        value={this.props.parameters.opt_format}
                        onChange={(e) => {
                            this.props.parameters.opt_format = e.target.value;
                            this.props.setParameters(this.props.parameters);
                        }}
                      >
                        <option value="jsonl">JSON</option>
                        <option value="csv">{T("CSV and JSON")}</option>
                      </Form.Control>
                    </Col>
                  </Form.Group>
                  <Form.Group as={Row}>
                    <Form.Label column sm="3">{T("Pause For Prompt")}</Form.Label>
                    <Col sm="8">
                      <Form.Check
                        type="checkbox"
                        onChange={(e) => {
                            if (e.currentTarget.checked) {
                                this.props.parameters.opt_prompt = "Y";
                            } else {
                                this.props.parameters.opt_prompt = "N";
                            }
                            this.props.setParameters(this.props.parameters);
                        }}
                        checked={this.props.parameters.opt_prompt === "Y"}
                      >
                      </Form.Check>
                    </Col>
                  </Form.Group>
                  <Form.Group as={Row}>
                    <Form.Label column sm="3">{T("Output Prefix")}</Form.Label>
                    <Col sm="8">
                      <Form.Control
                        as="input"
                        placeholder={T("Output filename prefix")}
                        spellCheck="false"
                        value={this.props.parameters.opt_output_directory}
                        onChange={e => {
                            this.props.parameters.opt_output_directory = e.target.value;
                            this.props.setParameters(this.props.parameters);
                        }}
                      />
                    </Col>
                  </Form.Group>

                </Form>
              </Modal.Body>
              <Modal.Footer>
                { this.props.paginator.makePaginator({
                    props: this.props,
                    step_name: "Offline Collector",
                }) }
              </Modal.Footer>
            </>
        );
    }
}

// The Offline collector has fewer resource controls than
// NewCollectionResources
class OfflineCollectionResources extends React.Component {
    static propTypes = {
        artifacts: PropTypes.array,
        resources: PropTypes.object,
        setResources: PropTypes.func,
        paginator: PropTypes.object,
    }

    state = {}

    isInvalid = () => {
        return this.state.invalid_1 || this.state.invalid_2 ||
            this.state.invalid_3 || this.state.invalid_4 ||
            this.state.invalid_5;
    }

    getTimeout = (artifacts) => {
        let timeout = 0;
        _.each(artifacts, (definition) => {
            let def_timeout = definition.resources && definition.resources.timeout;
            def_timeout = def_timeout || 0;

            if (def_timeout > timeout) {
                timeout = def_timeout;
            }
        });

        if (timeout === 0) {
            timeout = 600;
        }

        return timeout + " seconds";
    }

    getCpuLimit = (artifacts) => {
        let cpu_limit = 0;
        _.each(artifacts, (definition) => {
            let def_cpu_limit = definition.resources &&
                definition.resources.cpu_limit;
            def_cpu_limit = def_cpu_limit || 0;

            if (def_cpu_limit > cpu_limit) {
                cpu_limit = def_cpu_limit;
            }
        });

        if (cpu_limit === 0) {
            cpu_limit = "100";
        }

        return cpu_limit + "%";
    }

    render() {
        let resources = this.props.resources || {};
        return (
            <>
              <Modal.Header closeButton>
                <Modal.Title>{ T(this.props.paginator.title) }</Modal.Title>
              </Modal.Header>
              <Modal.Body>
                <Form>
                  <Form.Group as={Row}>
                    <Form.Label column sm="3">{T("CPU Limit Percent")}</Form.Label>
                    <Col sm="8">
                      <ValidatedInteger
                        placeholder={this.getCpuLimit(this.props.artifacts)}
                        value={resources.cpu_limit}
                        setInvalid={value => this.setState({
                            invalid_1: value})}
                        valid_func={value=>value >= 0 && value <=100}
                        setValue={value => {
                            this.props.setResources({cpu_limit: value});
                        }} />
                    </Col>
                  </Form.Group>

                  <Form.Group as={Row}>
                    <Form.Label column sm="3">{T("Max Execution Time in Seconds")}</Form.Label>
                    <Col sm="8">
                      <ValidatedInteger
                        placeholder={this.getTimeout(this.props.artifacts)}
                        value={resources.timeout}
                        setInvalid={value => this.setState({invalid_2: value})}
                        setValue={value => this.props.setResources({timeout: value})} />
                    </Col>
                  </Form.Group>

                  <Form.Group as={Row}>
                    <Form.Label column sm="3">Max Idle Time in Seconds</Form.Label>
                    <Col sm="8">
                      <ValidatedInteger
                        placeholder={T("If set collection will be terminated after this many seconds with no progress.")}
                        value={resources.progress_timeout}
                        setInvalid={value => this.setState({invalid_3: value})}
                        setValue={value => this.props.setResources({
                            progress_timeout: value})} />
                    </Col>
                  </Form.Group>

                </Form>
              </Modal.Body>
              <Modal.Footer>
                { this.props.paginator.makePaginator({
                    props: this.props,
                    isFocused: this.isInvalid(),
                }) }
              </Modal.Footer>
            </>
        );
    }
}



export default class OfflineCollectorWizard extends React.Component {
    static propTypes = {
        onResolve: PropTypes.func,
        onCancel: PropTypes.func,
    };

    state = {
        artifacts: [],
        parameters: {},
        collector_parameters: {
            target_os: "Windows",
            target: "ZIP",
            target_args: {
                // Common
                bucket: "",

                // For GCS Buckets
                GCSKey: "",

                // For S3 buckets.
                credentialsKey: "",
                credentialsSecret: "",
                region: "",
                endpoint: "",
                serverSideEncryption: "",
                kmsEncryptionKey: "",
                s3UploadRoot: "",

                // For Azure
                sas_url: "",
            },
            password: "",
            pubkey: "",
            encryption_scheme: "None",
            encryption_args: {
                public_key: "",
                password: ""
            },
            opt_level: 5,
            opt_output_directory: "",
            opt_format: "jsonl",
            opt_prompt: "N",
        },
        resources: {},
    }

    setArtifacts = (artifacts) => {
        this.setState({artifacts: artifacts});
    }

    setParameters = (params) => {
        this.setState({parameters: params});
    }

    // Build a request to the Server.Utils.CreateCollector artifact.
    prepareRequest = () => {
        let env = [];
        let request = {
            artifacts: ["Server.Utils.CreateCollector"],
            specs: [{artifact: "Server.Utils.CreateCollector", parameters: {env: env}}],
        };

        env.push({key: "OS",
                  value: this.state.collector_parameters.target_os});
        env.push({key: "artifacts", value: JSON.stringify(
            _.map(this.state.artifacts, (item) => item.name))});
        env.push({key: "parameters", value: JSON.stringify(this.state.parameters)});
        env.push({key: "target", value: this.state.collector_parameters.target});
        env.push({key: "target_args", value: JSON.stringify(
            this.state.collector_parameters.target_args)});
        env.push({key: "encryption_scheme", value: this.state.collector_parameters.encryption_scheme});
        env.push({key: "encryption_args", value: JSON.stringify(this.state.collector_parameters.encryption_args)});
        env.push({key: "opt_verbose", value: "Y"});
        env.push({key: "opt_banner", value: "Y"});
        env.push({key: "opt_prompt", value: this.state.collector_parameters.opt_prompt});
        env.push({key: "opt_admin", value: "Y"});
        env.push({key: "opt_tempdir", value: this.state.collector_parameters.opt_tempdir});
        env.push({key: "opt_level", value: this.state.collector_parameters.opt_level.toString()});
        env.push({key: "opt_output_directory", value: this.state.collector_parameters.opt_output_directory});
        env.push({key: "opt_progress_timeout", value: JSON.stringify(
            this.state.resources.progress_timeout)});
        env.push({key: "opt_timeout", value: JSON.stringify(
            this.state.resources.timeout)});
        env.push({key: "opt_cpu_limit", value: JSON.stringify(
            this.state.resources.cpu_limit)});
        env.push({key: "opt_format", value: this.state.collector_parameters.opt_format});

        return request;
    }

    launch = () => {
        this.props.onResolve(this.prepareRequest());
    }
    gotoStep = (step) => {
        return e=>{
            this.step.goToStep(step);
            e.preventDefault();
        };
    };

    gotoNextStep = () => {
        return e=>{
            this.step.nextStep();
            e.preventDefault();
        };
    }

    gotoPrevStep = () => {
        return e=>{
            this.step.previousStep();
            e.preventDefault();
        };
    }

    setResources = (resources) => {
        let new_resources = Object.assign(this.state.resources, resources);
        this.setState({resources: new_resources});
    }

    render() {
        let keymap = {
            GOTO_LAUNCH: "ctrl+l",
            NEXT_STEP: "ctrl+right",
            PREV_STEP: "ctrl+left",
        };
        let handlers={
            GOTO_LAUNCH: this.gotoStep(5),
            NEXT_STEP: this.gotoNextStep(),
            PREV_STEP: this.gotoPrevStep(),
        };

        let extra_tools = [ ];
        if ( this.state.collector_parameters &&
             this.state.collector_parameters.target_os) {
            extra_tools.push(tool_name_lookup[this.state.collector_parameters.target_os]);
        }

        return (
            <Modal show={true}
                   className="full-height"
                   dialogClassName="modal-90w"
                   enforceFocus={false}
                   scrollable={true}
                   onHide={this.props.onCancel}>
              <HotKeys keyMap={keymap}
                       handlers={handlers}>
                <ObserveKeys>
                  <StepWizard ref={n=>this.step=n}>
                    <NewCollectionSelectArtifacts
                      paginator={new OfflinePaginator(
                          "Select Artifacts",
                          "Create Offline collector: Select artifacts to collect",
                          (isFocused, step) => {
                              return isFocused;
                          })}
                      artifacts={this.state.artifacts}
                      artifactType="CLIENT"
                      setParameters={this.setParameters}
                      setArtifacts={this.setArtifacts}/>

                    <NewCollectionConfigParameters
                      parameters={this.state.parameters}
                      setParameters={this.setParameters}
                      artifacts={this.state.artifacts}
                      setArtifacts={this.setArtifacts}
                      paginator={new OfflinePaginator(
                          "Configure Parameters",
                          "Create Offline Collector: Configure artifact parameters")}
                    />

                    <OfflineCollectorParameters
                      setParameters={(x) => this.setState({collector_parameters: x})}
                      parameters={this.state.collector_parameters}
                      paginator={new OfflinePaginator(
                          "Configure Collection",
                          "Create Offline Collector: Configure collector")}
                    />

                    <OfflineCollectionResources
                      artifacts={this.state.artifacts}
                      resources={this.state.resources}
                      paginator={new OfflinePaginator(
                          "Specify Resources",
                          "New Collection: Specify Resources")}
                      setResources={this.setResources} />

                    <NewCollectionRequest
                      paginator={new OfflinePaginator(
                          "Review",
                          "Create Offline Collector: Review request")}
                      request={this.prepareRequest()} />

                    <NewCollectionLaunch
                      artifacts={this.state.artifacts}
                      tools={extra_tools}
                      paginator={new OfflinePaginator(
                          "Launch",
                          "Create Offline Collector: Create collector")}
                      launch={this.launch} />

                  </StepWizard>
                </ObserveKeys>
              </HotKeys>
            </Modal>
        );
    }
};
