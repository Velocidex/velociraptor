import './new-collection.css';

import React from 'react';
import PropTypes from 'prop-types';
import _ from 'lodash';

import Alert from 'react-bootstrap/Alert';
import Button from 'react-bootstrap/Button';
import Modal from 'react-bootstrap/Modal';
import filterFactory, { textFilter } from 'react-bootstrap-table2-filter';
import BootstrapTable from 'react-bootstrap-table-next';
import Pagination from 'react-bootstrap/Pagination';
import Form from 'react-bootstrap/Form';
import Row from 'react-bootstrap/Row';
import VeloReportViewer from "../artifacts/reporting.js";
import ButtonGroup from 'react-bootstrap/ButtonGroup';

import Spinner from '../utils/spinner.js';
import Col from 'react-bootstrap/Col';

import StepWizard from 'react-step-wizard';
import VeloForm from '../forms/form.js';

import ValidatedInteger from "../forms/validated_int.js";

import VeloAce from '../core/ace.js';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { HotKeys, ObserveKeys } from "react-hotkeys";
import { requestToParameters } from "./utils.js";

import api from '../core/api-service.js';


class PaginationBuilder {
    PaginationSteps = ["Select Artifacts", "Configure Parameters",
                       "Specify Resources", "Review", "Launch"];

    constructor(name, title, shouldFocused) {
        this.title = title;
        this.name = name;
        if (shouldFocused) {
            this.shouldFocused = shouldFocused;
        }
    }

    title = ""
    name = ""
    shouldFocused = (isFocused, step) => isFocused;

    // A common function to create the modal paginator between wizard
    // pages.
    // onBlur is a function that will be called when we leave the current page.

    // If isFocused is true, the current page is focused and all other
    // navigators are disabled.
    makePaginator = (spec) => {
        let {props, onBlur, isFocused} = spec;
        return <Col md="12">
                 <Pagination>
                   { _.map(this.PaginationSteps, (step, i) => {
                       let idx = i;
                       if (step === this.name) {
                           return <Pagination.Item
                                    active key={idx}>
                                    {step}
                                  </Pagination.Item>;
                       };
                       return <Pagination.Item
                                onClick={() => {
                                    if (onBlur) {onBlur();};
                                    props.goToStep(idx + 1);
                                }}
                                disabled={this.shouldFocused(isFocused, step)}
                                key={idx}>
                                {step}
                              </Pagination.Item>;
                   })}
                 </Pagination>
               </Col>;
    }
}

// Add an artifact to the list of artifacts.
const add_artifact = (artifacts, new_artifact) => {
    // Remove it from the list if it exists already
    let result = _.filter(artifacts, (x) => x.name !== new_artifact.name);

    // Push the new artifact to the end of the list.
    result.push(new_artifact);
    return result;
};

// Remove the named artifact from the list of artifacts
const remove_artifact = (artifacts, name) => {
    return _.filter(artifacts, (x) => x.name !== name);
};


class NewCollectionSelectArtifacts extends React.Component {
    static propTypes = {
        // A list of artifact descriptors that are selected.
        artifacts: PropTypes.array,

        // Update the wizard's artifacts list.
        setArtifacts: PropTypes.func,
        paginator: PropTypes.object,

        // Artifact type CLIENT, SERVER, CLIENT_EVENT, SERVER_EVENT
        artifactType: PropTypes.string,
    };

    state = {
        selectedDescriptor: "",

        // A list of descriptors that match the search term.
        matchingDescriptors: [],

        loading: false,

        initialized_from_parent: false,
    }

    componentDidMount = () => {
        this.doSearch("...");
        const el = document.getElementById("text-filter-column-name-search-for-artifact-input");
        if (el) {
            setTimeout(()=>el.focus(), 1000);
        };
    }

    onSelect = (row, isSelect) => {
        let new_artifacts = [];
        if (isSelect) {
            new_artifacts = add_artifact(this.props.artifacts, row);
        } else {
            new_artifacts = remove_artifact(this.props.artifacts, row.name);
        }
        this.props.setArtifacts(new_artifacts);
        this.setState({selectedDescriptor: row});

    }

    onSelectAll = (isSelect, rows) => {
        _.each(rows, (row) => this.onSelect(row, isSelect));
    }

    updateSearch = (type, filters) => {
        let value = filters && filters.filters && filters.filters.name &&
            filters.filters.name.filterVal;
        if (!value) {
            this.doSearch("...");
            return;
        }
        this.doSearch(value);
    }

    doSearch = (value) => {
        this.setState({loading: true});
        api.get("v1/GetArtifacts", {
            type: this.props.artifactType,
            search_term: value}).then((response) => {
            let items = response.data.items || [];
            this.setState({matchingDescriptors: items, loading: false});
        });
    }

    toggleSelection = (row, e) => {
        for(let i=0; i<this.props.artifacts.length; i++) {
            let selected_artifact = this.props.artifacts[i];
            if (selected_artifact.name === row.name) {
                this.onSelect(row, false);
                return;
            }
        }
        this.onSelect(row, true);
        e.stopPropagation();
    }

    render() {
        let columns = [{dataField: "name", text: "",
                        formatter: (cell, row) => {
                            return <Button tabIndex={0} variant="link"
                                           onClick={(e)=>this.toggleSelection(row, e)}
                                   >{cell}</Button>;
                        },
                        filter: textFilter({
                            id: "search-for-artifact-input",
                            placeholder: "Search for artifacts...",
                        })}];

        let selectRow = {mode: "checkbox",
                         clickToSelect: true,
                         classes: "row-selected",
                         selected: _.map(this.props.artifacts, x=>x.name),
                         onSelect: this.onSelect,
                         hideSelectColumn: true,
                         onSelectAll: this.onSelectAll,
                        };
        return (
            <>
              <Modal.Header closeButton>
                <Modal.Title>{ this.props.paginator.title }</Modal.Title>
              </Modal.Header>

              <Modal.Body>
                <div className="row new-artifact-page">
                  <div className="col-6 new-artifact-search-table selectable">
                    { this.state.loading && <Spinner loading={this.state.loading}/> }
                    <BootstrapTable
                      hover
                      condensed
                      bootstrap4
                      remote={ { filter: true } }
                      filter={ filterFactory() }
                      keyField="name"
                      data={this.state.matchingDescriptors}
                      columns={columns}
                      selectRow={ selectRow }
                      onTableChange={ this.updateSearch }
                    />

                  </div>
                  <div name="ArtifactInfo" className="col-6 new-artifact-description">
                    { this.state.selectedDescriptor &&
                      <VeloReportViewer
                        artifact={this.state.selectedDescriptor.name}
                        type="ARTIFACT_DESCRIPTION"
                      />
                    }
                  </div>
                </div>

              </Modal.Body>
              <Modal.Footer>
                { this.props.paginator.makePaginator({
                    props: this.props,
                    isFocused: _.isEmpty(this.props.artifacts),
                }) }
              </Modal.Footer>
            </>
        );
    }
};

class NewCollectionConfigParameters extends React.Component {
    static propTypes = {
        request: PropTypes.object,
        artifacts: PropTypes.array,
        setArtifacts: PropTypes.func.isRequired,
        parameters: PropTypes.object,
        setParameters: PropTypes.func.isRequired,
        paginator: PropTypes.object,
    };

    setValue = (artifact, name, value) => {
        let parameters = this.props.parameters;
        let artifact_parameters = parameters[artifact] || {};
        artifact_parameters[name] = value;
        parameters[artifact] = artifact_parameters;
        this.props.setParameters(parameters);
    }

    removeArtifact = (name) => {
        this.props.setArtifacts(remove_artifact(this.props.artifacts, name));
    }

    render() {
        const expandRow = {
            expandHeaderColumnRenderer: ({ isAnyExpands }) => {
                if (isAnyExpands) {
                    return <b>-</b>;
                }
                return <b>+</b>;
            },
            expandColumnRenderer: ({ expanded, rowKey }) => {
                if (expanded) {
                    return (
                        <b key={rowKey}>-</b>
                    );
                }
                return (<ButtonGroup>
                          <Button variant="outline-default"><FontAwesomeIcon icon="wrench"/></Button>
                          <Button variant="outline-default" onClick={
                              () => this.props.setArtifacts(remove_artifact(
                                  this.props.artifacts, rowKey))} >
                            <FontAwesomeIcon icon="trash"/>
                          </Button>
                        </ButtonGroup>
                );
            },
            showExpandColumn: true,
            renderer: artifact => {
                return _.map(artifact.parameters || [], (param, idx) => {
                    let artifact_parameters = this.props.parameters[artifact.name] || {};
                    let value = artifact_parameters[param.name];
                    // Only set default value if the parameter is not
                    // defined. If it is an empty string then so be
                    // it.
                    if (_.isUndefined(value)) {
                        value = param.default || "";
                    }

                    return (
                        <VeloForm param={param} key={idx}
                                  value={value}
                                  setValue={(value) => this.setValue(
                                      artifact.name,
                                      param.name,
                                      value)}/>
                    );
                });
            }
        };
        return (
            <>
              <Modal.Header closeButton>
                <Modal.Title>{ this.props.paginator.title }</Modal.Title>
              </Modal.Header>

              <Modal.Body className="new-collection-parameter-page selectable">
                { !_.isEmpty(this.props.artifacts) ?
                  <BootstrapTable
                    keyField="name"
                    expandRow={ expandRow }
                    columns={[{dataField: "name", text: "Artifact"},
                              {dataField: "parameter", text: "", hidden: true}]}
                    data={this.props.artifacts} /> :
                 <div className="no-content">
                   No artifacts configured. Please add some artifacts to collect
                 </div>
                }
              </Modal.Body>
              <Modal.Footer>
                { this.props.paginator.makePaginator({
                    props: this.props,
                }) }
              </Modal.Footer>
            </>
        );
    };
}

class NewCollectionResources extends React.Component {
    static propTypes = {
        resources: PropTypes.object,
        setResources: PropTypes.func,
        paginator: PropTypes.object,
    }

    state = {}

    isInvalid = () => {
        return this.state.invalid_1 || this.state.invalid_2 ||
            this.state.invalid_3 || this.state.invalid_4;
    }

    render() {
        let resources = this.props.resources || {};
        return (
            <>
              <Modal.Header closeButton>
                <Modal.Title>{ this.props.paginator.title }</Modal.Title>
              </Modal.Header>
              <Modal.Body>
                <Form>
                  <Form.Group as={Row}>
                    <Form.Label column sm="3">Ops/Sec</Form.Label>
                    <Col sm="8">
                      <ValidatedInteger
                        placeholder="Unlimited"
                        value={resources.ops_per_second}
                        setInvalid={value => this.setState({invalid_1: value})}
                        setValue={value => this.props.setResources({ops_per_second: value})} />
                    </Col>
                  </Form.Group>

                  <Form.Group as={Row}>
                    <Form.Label column sm="3">Max Execution Time in Seconds</Form.Label>
                    <Col sm="8">
                      <ValidatedInteger
                        placeholder="600"
                        value={resources.timeout}
                        setInvalid={value => this.setState({invalid_2: value})}
                        setValue={value => this.props.setResources({timeout: value})} />
                    </Col>
                  </Form.Group>

                  <Form.Group as={Row}>
                    <Form.Label column sm="3">Max Rows</Form.Label>
                    <Col sm="8">
                      <ValidatedInteger
                        placeholder="Unlimited"
                        value={resources.max_rows}
                        setInvalid={value => this.setState({invalid_3: value})}
                        setValue={value => this.props.setResources({max_rows: value})} />
                    </Col>
                  </Form.Group>

                  <Form.Group as={Row}>
                    <Form.Label column sm="3">Max Mb Uploaded</Form.Label>
                    <Col sm="8">
                      <ValidatedInteger
                        placeholder="Unlimited"
                        value={resources.max_mbytes}
                        setInvalid={value => this.setState({invalid_4: value})}
                        setValue={value => this.props.setResources({max_mbytes: value})} />
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

class NewCollectionRequest extends React.Component {
    static propTypes = {
        request: PropTypes.object,
        paginator: PropTypes.object,
    }

    render() {
        let serialized =  JSON.stringify(this.props.request, null, 2);
        return (
            <>
              <Modal.Header closeButton>
                <Modal.Title>{ this.props.paginator.title }</Modal.Title>
              </Modal.Header>
              <Modal.Body>
                <VeloAce text={serialized}
                         options={{readOnly: true,
                                   autoScrollEditorIntoView: true,
                                   wrap: true,
                                   maxLines: 1000}} />
              </Modal.Body>
              <Modal.Footer>
                { this.props.paginator.makePaginator({
                    props: this.props,
                }) }
              </Modal.Footer>
            </>
        );
    }
}


class NewCollectionLaunch extends React.Component {
    static propTypes = {
        artifacts: PropTypes.array,
        launch: PropTypes.func,
        isActive: PropTypes.bool,
        paginator: PropTypes.object,
        tools: PropTypes.array,
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        if (this.props.isActive && !prevProps.isActive) {
            this.processTools();
        }
    }

    state = {
        tools: {},
        current_tool: "",
        current_error: "",
        checking_tools: [],
    }

    processTools = () => {
        // Extract all the tool names from the artifacts we collect.
        let tools = {};
        _.each(this.props.tools, t=>{
            tools[t] = true;
        });

        _.each(this.props.artifacts, a=>{
            _.each(a.tools, t=>{
                tools[t.name] = t;
            });
        });

        this.setState({
            tools: {},
            current_tool: "",
            current_error: "",
            checking_tools: [],
        });
        this.checkTools(tools);
    }

    checkTools = tools_dict => {
        let tools = Object.assign({}, tools_dict);
        let tool_names = Object.keys(tools);

        if (_.isEmpty(tool_names)) {
            this.props.launch();
            return;
        }

        // Recursively call this function with the first tool.
        var first_tool = tool_names[0];

        // Clear it.
        delete tools[first_tool];

        // Inform the user we are checking this tool.
        let checking_tools = this.state.checking_tools;
        checking_tools.push(first_tool);
        this.setState({current_tool: first_tool, checking_tools: checking_tools});

        api.get('v1/GetToolInfo', {
            name: first_tool,
            materialize: true,
        }).then(response=>{
            this.checkTools(tools);
        }).catch(err=>{
            let data = err.response && err.response.data && err.response.data.message;
            this.setState({current_error: data});
        });
    }

    render() {
        return <>
                 <Modal.Header closeButton>
                   <Modal.Title>{ this.props.paginator.title }</Modal.Title>
                 </Modal.Header>
                 <Modal.Body>
                   {_.map(this.state.checking_tools, (name, idx)=>{
                       let variant = "success";
                       if (name === this.state.current_tool) {
                           variant = "primary";

                           if (this.state.current_error) {
                               return (
                                   <Alert key={idx} variant="danger" className="text-center">
                                     Checking { name }: {this.state.current_error}
                                   </Alert>
                               );
                           }
                       }

                       return (
                           <Alert key={idx} variant={variant} className="text-center">
                              Checking { name }
                           </Alert>
                       );
                   })}
                 </Modal.Body>
                 <Modal.Footer>
                   { this.props.paginator.makePaginator({
                       props: this.props,
                       onBlur: ()=>this.setState({checking_tools: []}),
                   }) }
                 </Modal.Footer>
               </>;
    }
}

class NewCollectionWizard extends React.Component {
    static propTypes = {
        baseFlow: PropTypes.object,
        onResolve: PropTypes.func,
        onCancel: PropTypes.func,
    }

    componentDidMount = () => {
        this.initializeFromBaseFlow();
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        if (!this.state.original_flow && this.props.baseFlow) {
            this.initializeFromBaseFlow();
        }
    }

    initializeFromBaseFlow = () => {
        let request = this.props.baseFlow && this.props.baseFlow.request;
        if (!request || !request.artifacts) {
            return;
        }

        let resources = {
            ops_per_second: request.ops_per_second,
            timeout: request.timeout,
            max_rows: request.max_rows,
            max_mbytes: (request.max_upload_bytes || 0) / 1024 / 1024,
        };

        this.setState({
            original_flow: this.props.baseFlow,
            resources: resources,
            initialized_from_parent: true,
        });

        // Resolve the artifacts from the request into a list of descriptors.
        api.get("v1/GetArtifacts", {names: request.artifacts}).then(response=>{
                if (response && response.data &&
                    response.data.items && response.data.items.length) {

                    this.setState({artifacts: [...response.data.items],
                                   parameters: requestToParameters(request)});
                }});
    }

    state = {
        original_flow: null,

        // A list of artifact descriptors we have selected so far.
        artifacts: [],

        // A key/value mapping of edited parameters by the user. Key -
        // artifact name, value - parameters dict
        parameters: {},

        resources: {},

        initialized_from_parent: false,
    }

    setArtifacts = (artifacts) => {
        this.setState({artifacts: artifacts});
    }

    setParameters = (params) => {
        this.setState({parameters: params});
    }

    setResources = (resources) => {
        let new_resources = Object.assign(this.state.resources, resources);
        this.setState({resources: new_resources});
    }

    // Let our caller know the artifact request we created.
    launch = () => {
        this.props.onResolve(this.prepareRequest());
    }

    prepareRequest = () => {
        let specs = [];
        let artifacts = [];
        _.each(this.state.artifacts, (item) => {
            let spec = {
                artifact: item.name,
                parameters: {env: []},
            };

            _.each(this.state.parameters[item.name], (v, k) => {
                spec.parameters.env.push({key: k, value: v});
            });
            specs.push(spec);
            artifacts.push(item.name);
        });

        let result = {
            artifacts: artifacts,
            specs: specs,
        };

        if (this.state.resources.ops_per_second) {
            result.ops_per_second = this.state.resources.ops_per_second;
        }

        if (this.state.resources.timeout) {
            result.timeout = this.state.resources.timeout;
        }

        if (this.state.resources.max_rows) {
            result.max_rows = this.state.resources.max_rows;
        }

        if (this.state.resources.max_mbytes) {
            result.max_upload_bytes = this.state.resources.max_mbytes * 1024 * 1024;
        }

        return result;
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

    render() {
        let request = this.state.original_flow && this.state.original_flow.request;
        let client_id = this.props.client && this.props.client.client_id;
        let type = "CLIENT";
        if (!client_id || client_id==="server") {
            type="SERVER";
        }

        let keymap = {
            GOTO_ARTIFACTS: "alt+a",
            GOTO_PARAMETERS: "alt+p",
            GOTO_PREVIEW: "alt+v",
            GOTO_RESOURCES: "alt+r",
            GOTO_LAUNCH: "ctrl+l",
            NEXT_STEP: "ctrl+right",
            PREV_STEP: "ctrl+left",
        };
        let handlers={
            GOTO_ARTIFACTS: this.gotoStep(1),
            GOTO_PARAMETERS: this.gotoStep(2),
            GOTO_RESOURCES: this.gotoStep(3),
            GOTO_PREVIEW: this.gotoStep(4),
            GOTO_LAUNCH: this.gotoStep(5),
            NEXT_STEP: this.gotoNextStep(),
            PREV_STEP: this.gotoPrevStep(),
        };
        return (
            <Modal show={true}
                   className="full-height"
                   dialogClassName="modal-90w"
                   scrollable={true}
                   onHide={this.props.onCancel}>
              <HotKeys keyMap={keymap} handlers={handlers}><ObserveKeys>
                  <StepWizard ref={n=>this.step=n}>
                  <NewCollectionSelectArtifacts
                    artifacts={this.state.artifacts}
                    artifactType={type}
                    paginator={new PaginationBuilder(
                        "Select Artifacts",
                        "New Collection: Select Artifacts to collect")}
                    setArtifacts={this.setArtifacts}/>

                  <NewCollectionConfigParameters
                    parameters={this.state.parameters}
                    setParameters={this.setParameters}
                    artifacts={this.state.artifacts}
                    setArtifacts={this.setArtifacts}
                    paginator={new PaginationBuilder(
                        "Configure Parameters",
                        "New Collection: Configure Parameters")}
                    request={request}/>

                  <NewCollectionResources
                    resources={this.state.resources}
                    paginator={new PaginationBuilder(
                        "Specify Resources",
                        "New Collection: Specify Resources")}
                    setResources={this.setResources} />

                  <NewCollectionRequest
                    paginator={new PaginationBuilder(
                        "Review",
                        "New Collection: Review request")}
                    request={this.prepareRequest()} />

                    <NewCollectionLaunch
                      artifacts={this.state.artifacts}
                      paginator={new PaginationBuilder(
                          "Launch",
                          "New Collection: Launch collection")}
                      launch={this.launch} />
                </StepWizard>
                </ObserveKeys></HotKeys>
            </Modal>
        );
    }
}


export {
    NewCollectionWizard as default,
    NewCollectionSelectArtifacts,
    NewCollectionConfigParameters,
    NewCollectionResources,
    NewCollectionRequest,
    NewCollectionLaunch,
    PaginationBuilder
 }
