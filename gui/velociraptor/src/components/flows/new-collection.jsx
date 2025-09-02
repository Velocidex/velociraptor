import './new-collection.css';

import React from 'react';
import PropTypes from 'prop-types';
import _ from 'lodash';
import T from '../i8n/i8n.jsx';
import Alert from 'react-bootstrap/Alert';
import Button from 'react-bootstrap/Button';
import Modal from 'react-bootstrap/Modal';
import Pagination from 'react-bootstrap/Pagination';
import Form from 'react-bootstrap/Form';
import Row from 'react-bootstrap/Row';
import VeloReportViewer from "../artifacts/reporting.jsx";
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Select from 'react-select';
import NewCollectionConfigParameters from './new-collections-parameters.jsx';
import Dropdown from 'react-bootstrap/Dropdown';
import Spinner from '../utils/spinner.jsx';
import Col from 'react-bootstrap/Col';
import Table  from 'react-bootstrap/Table';

import StepWizard from 'react-step-wizard';

import ValidatedInteger from "../forms/validated_int.jsx";

import VeloAce from '../core/ace.jsx';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { HotKeys, ObserveKeys } from "react-hotkeys";
import { requestToParameters, runArtifact } from "./utils.jsx";

import api from '../core/api-service.jsx';
import {CancelToken} from 'axios';

class PaginationBuilder {
    PaginationSteps = [T("Select Artifacts"), T("Configure Parameters"),
                       T("Specify Resources"), T("Review"), T("Launch")];

    constructor(name, title, shouldFocused) {
        this.title = title;
        this.name = name;
        if (shouldFocused) {
            this.shouldFocused = shouldFocused;
        }
    }

    active = false
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
        this.active = this.name === this.PaginationSteps[props.currentStep-1];
        return <Col md="12">
                 <Pagination>
                   { _.map(this.PaginationSteps, (step, i) => {
                       let idx = i;
                       if (step === this.name) {
                           return <Pagination.Item
                                    active key={idx}>
                                    {T(step)}
                                  </Pagination.Item>;
                       };
                       return <Pagination.Item
                                onClick={() => {
                                    if (onBlur) {onBlur();};
                                    props.goToStep(idx + 1);
                                }}
                                disabled={this.shouldFocused(isFocused, step)}
                                key={idx}>
                                {T(step)}
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
    inputRef = React.createRef(null);

    static propTypes = {
        // A list of artifact descriptors that are selected.
        artifacts: PropTypes.array,

        // Update the wizard's artifacts list.
        setArtifacts: PropTypes.func.isRequired,
        setParameters: PropTypes.func,
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

        favorites: [],
        favorite_options: [],
        showFavoriteSelector: false,

        current_favorite: null,

        active: false,
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
        this.focusSearch();

        // Prepare the debounced function in advance.
        this._doSearch = _.debounce((value)=>{
            api.post("v1/GetArtifacts", {
                type: this.props.artifactType,
                fields: {
                    name: true,
                },
                search_term: this.state.filter,
            }, this.source.token).then((response) => {
                    let items = response.data.items || [];
                    this.setState({matchingDescriptors: items, loading: false});
                });
        }, 400);
        this.doSearch("...");
    }

    componentWillUnmount() {
        this.source.cancel();
    }

    // Trigger a selection of the first artifacts when the list is
    // added. This will render the artifact description view.
    componentDidUpdate = (prevProps, prevState, rootNode) => {
        if(_.isEmpty(prevProps.artifacts) && !_.isEmpty(this.props.artifacts)) {
            this.onSelect(this.props.artifacts[0], true);
        }

        if(this.props.paginator.active && !this.state.active) {
            this.focusSearch();
            this.setState({active: true});
        }

        if(!this.props.paginator.active && this.state.active) {
            this.setState({active: false});
        }
    }

    focusSearch() {
        const input = this.inputRef.current;

        // Focus the input box once all the form is rendered.
        setTimeout(function() {
            input.select();
        }, 400);
    }

    onSelect = (row, isSelect) => {
        // The row contains only the name so we need to make another
        // request to fetch the full definition.
        let name = row["name"];
        api.post("v1/GetArtifacts", {
            names: [name],
        }, this.source.token).then(response=>{
            let items = response.data.items || [];
            if (_.isEmpty(items)) {
                return;
            }
            let definition = items[0];;
            let new_artifacts = [];
            if (isSelect) {
                new_artifacts = add_artifact(this.props.artifacts, definition);
            } else {
                new_artifacts = remove_artifact(this.props.artifacts, row.name);
            }
            this.props.setArtifacts(new_artifacts);
            this.setState({selectedDescriptor: definition});
        });
    }

    onSelectAll = (isSelect, rows) => {
        _.each(rows, (row) => this.onSelect(row, isSelect));
    }


    doSearch = (value) => {
        this.setState({
            filter: value,
        });

        this._doSearch(value);
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

    fetchFavorites = () => {
        api.get('v1/GetUserFavorites', {
            type: this.props.artifactType,
        }, this.source.token).then(response=>{
            this.setState({
                favorites: response.data.items,
                favorite_options: _.map(response.data.items, x=>{
                    return {value: x.name, label: x.name,
                            color: "#00B8D9", isFixed: true};
                })});
        });
    }

    deleteFavorite = (name, type)=>{
        this.setState({loading: true});
        runArtifact("server",   // This collection happens on the server.
                    "Server.Utils.DeleteFavoriteFlow",
                    {
                        Name: name,
                        Type: type,
                    }, ()=>{
                        this.fetchFavorites();
                        this.setState({loading: false});
                    }, this.source.token);
    }

    setFavorite = (spec) => {
        // First set all the artifacts in the spec.
        let artifacts = [];
        _.each(spec, x=>artifacts.push(x.artifact));

        if(_.isEmpty(artifacts)) {
            return;
        }

        api.post("v1/GetArtifacts", {
            type: this.props.artifactType,
            names: artifacts}, this.source.token).then((response) => {
                let items = response.data.items || [];
                this.setState({
                    selectedDescriptor: items[0],
                    matchingDescriptors: items,
                });

                // Set the artifacts.
                this.props.setArtifacts(items);
                if (this.props.setParameters) {
                    this.props.setParameters(requestToParameters({specs: spec}));
                };
            }, this.source.token);
    }

    setFavorite_ = (selection) => {
        _.each(this.state.favorites, fav=>{
            if(selection.value === fav.name) {
                this.setFavorite(fav.spec);
                this.setState({current_favorite: {name: fav.name, type: fav.type}});
            }
        });
        return true;
    }

    render() {
        return (
            <>
              <Modal.Header closeButton>
                <Modal.Title className="flex-fill">{ T(this.props.paginator.title) }
                  <ButtonGroup className="float-right">
                    { this.state.showFavoriteSelector &&
                      <Select
                        className="favorites"
                        classNamePrefix="velo"
                        options={this.state.favorite_options}
                        onChange={this.setFavorite_}
                        placeholder={T("Favorite Name")}
                        spellCheck="false"
                      />
                    }
                    { this.state.current_favorite ?
                      <Button variant="default"
                              onClick={(e)=>{
                                  this.deleteFavorite(
                                      this.state.current_favorite.name,
                                      this.state.current_favorite.type,
                                  );
                                  this.setState({
                                      current_favorite: null,
                                  });
                              }}
                              className="favorite-button float-right">
                        <FontAwesomeIcon icon="trash"/>
                      </Button>
                      :
                      <Button variant="default"
                              onClick={(e)=>{
                                  this.fetchFavorites();
                                  this.setState({
                                      showFavoriteSelector: !this.state.showFavoriteSelector
                                  });
                              }}
                              className="favorite-button float-right">
                        <FontAwesomeIcon icon="heart"/>
                      </Button>
                    }
                  </ButtonGroup>
                </Modal.Title>
              </Modal.Header>

              <Modal.Body>
                <div className="row new-artifact-page">
                  <div className="col-4 new-artifact-search-table selectable">
                    { this.state.loading && <Spinner loading={this.state.loading}/> }
                    <Table>
                      <thead>
                        <tr>
                          <th>
                            <Form onSubmit={e=>{
                                e.preventDefault();
                                return false;
                            }}>
                              <Form.Control
                                type="text"
                                ref={this.inputRef}
                                onChange={e=>this.doSearch(e.target.value)}
                                placeholder={T("Search for artifacts...")}
                              />
                            </Form>
                          </th>
                        </tr>
                      </thead>
                      <tbody>
                        {_.map(this.state.matchingDescriptors, (row, i)=>{
                            let classname = "";
                            _.each(this.props.artifacts, x=>{
                                if(x.name===row.name) {
                                    classname = "row-selected";
                                }
                            });
                            return <tr key={i} className={classname}>
                                     <td>
                                       <Button
                                         variant="link"
                                         onClick={(e)=>this.toggleSelection(row, e)} >
                                         {row.name}
                                       </Button>
                                     </td></tr>;
                        })}
                      </tbody>
                    </Table>
                  </div>
                  <div name="ArtifactInfo" className="col-8 new-artifact-description">
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

class NewCollectionResources extends React.Component {
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

        return timeout + "s per artifact";
    }

    getMaxUploadBytes = (artifacts) => {
        let max_upload_bytes = 0;
        _.each(artifacts, (definition) => {
            let def_max_upload_bytes = definition.resources &&
                definition.resources.max_upload_bytes;
            def_max_upload_bytes = def_max_upload_bytes || 0;

            if (def_max_upload_bytes > max_upload_bytes) {
                max_upload_bytes = def_max_upload_bytes;
            }

        });

        let max_mbytes = max_upload_bytes / 1024 / 1024;

        if (max_mbytes === 0) {
            return "1 Gb";
        }
        return max_mbytes.toFixed(2) + " Mb";
    }

    getMaxRows = (artifacts) => {
        let max_rows = 0;
        _.each(artifacts, (definition) => {
            let def_max_rows = definition.resources && definition.resources.max_rows;
            def_max_rows = def_max_rows || 0;

            if (def_max_rows > max_rows) {
                max_rows = def_max_rows;
            }
        });

        if (max_rows === 0) {
            return "1,000,000 rows";
        }

        return max_rows + " rows";
    }

    getOpsPerSecond = (artifacts) => {
        let ops_per_second = 0;
        _.each(artifacts, (definition) => {
            let def_ops_per_sec = definition.resources &&
                definition.resources.ops_per_second;
            def_ops_per_sec = def_ops_per_sec || 0;

            if (def_ops_per_sec > ops_per_second) {
                ops_per_second = def_ops_per_sec;
            }
        });

        if (ops_per_second === 0) {
            return T("Unlimited");
        }

        return T("X per second", ops_per_second);
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

    getIopsLimit = (artifacts) => {
        let iops_limit = 0;
        _.each(artifacts, (definition) => {
            let def_iops_limit = definition.resources &&
                definition.resources.iops_limit;
            def_iops_limit = def_iops_limit || 0;

            if (def_iops_limit > iops_limit) {
                iops_limit = def_iops_limit;
            }
        });

        if (iops_limit === 0) {
            return T("Unlimited");
        }

        return T("X per second", iops_limit);
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
                        value={resources.cpu_limit || undefined}
                        valid_func={value=>value >= 0 && value <=100}
                        setInvalid={value => this.setState({
                            invalid_1: value})}
                        setValue={value => this.props.setResources({
                            cpu_limit: value
                        })} />
                    </Col>
                  </Form.Group>
                  <Form.Group as={Row}>
                    <Form.Label column sm="3">{T("IOps/Sec")}</Form.Label>
                    <Col sm="8">
                      <ValidatedInteger
                        placeholder={this.getIopsLimit(this.props.artifacts)}
                        value={resources.iops_limit || undefined}
                        setInvalid={value => this.setState({invalid_1: value})}
                        setValue={value => this.props.setResources({
                            iops_limit: value
                      })} />
                    </Col>
                  </Form.Group>

                  <Form.Group as={Row}>
                    <Form.Label column sm="3">{T("Max Execution Time in Seconds")}</Form.Label>
                    <Col sm="8">
                      <ValidatedInteger
                        placeholder={this.getTimeout(this.props.artifacts)}
                        value={resources.timeout || undefined}
                        setInvalid={value => this.setState({invalid_2: value})}
                        setValue={value => this.props.setResources({timeout: value})} />
                    </Col>
                  </Form.Group>

                  <Form.Group as={Row}>
                    <Form.Label column sm="3">{T("Max Idle Time in Seconds")}</Form.Label>
                    <Col sm="8">
                      <ValidatedInteger
                        placeholder={T("If set collection will be terminated after this many seconds with no progress.")}
                        value={resources.progress_timeout || undefined}
                        setInvalid={value => this.setState({invalid_3: value})}
                        setValue={value => this.props.setResources({
                            progress_timeout: value})} />
                    </Col>
                  </Form.Group>

                  <Form.Group as={Row}>
                    <Form.Label column sm="3">{T("Max Rows")}</Form.Label>
                    <Col sm="8">
                      <ValidatedInteger
                        placeholder={this.getMaxRows(this.props.artifacts)}
                        value={resources.max_rows || undefined}
                        setInvalid={value => this.setState({invalid_4: value})}
                        setValue={value => this.props.setResources({max_rows: value})} />
                    </Col>
                  </Form.Group>

                  <Form.Group as={Row}>
                    <Form.Label column sm="3">{T("Max Logs")}</Form.Label>
                    <Col sm="8">
                      <ValidatedInteger
                        placeholder={"100000"}
                        value={resources.max_logs || undefined}
                        setInvalid={value => this.setState({invalid_6: value})}
                        setValue={value => this.props.setResources({max_logs: value})} />
                    </Col>
                  </Form.Group>

                  <Form.Group as={Row}>
                    <Form.Label column sm="3">{T("Max MB uploaded")}</Form.Label>
                    <Col sm="8">
                      <ValidatedInteger
                        placeholder={this.getMaxUploadBytes(this.props.artifacts)}
                        value={resources.max_mbytes || undefined}
                        setInvalid={value => this.setState({invalid_5: value})}
                        setValue={value => this.props.setResources({max_mbytes: value})} />
                    </Col>
                  </Form.Group>

                  <Form.Group as={Row}>
                    <Form.Label column sm="3">{T("Trace Frequency Seconds")}</Form.Label>
                    <Col sm="8">
                      <Dropdown>
                        <Dropdown.Toggle
                          variant="default"
                          className="trace-selector">
                          {this.state.trace ||
                           T("To enable tracing, specify trace update frequency in seconds ")}
                        </Dropdown.Toggle>
                        <Dropdown.Menu>
                          <Dropdown.Item onClick={()=>{
                              this.props.setResources({trace_freq_sec: 0});
                              this.setState({trace: ""});
                            }} >
                            {T("Disable tracing")}
                          </Dropdown.Item>
                          <Dropdown.Item onClick={()=>{
                              this.props.setResources({trace_freq_sec: 10});
                              this.setState({trace: T("Every 10 Seconds")});
                            }} >
                            {T("Every 10 Seconds")}
                          </Dropdown.Item>
                          <Dropdown.Item onClick={()=>{
                              this.props.setResources({trace_freq_sec: 30});
                              this.setState({trace: T("Every 30 Seconds")});
                            }} >
                            {T("Every 30 Seconds")}
                          </Dropdown.Item>
                          <Dropdown.Item onClick={()=>{
                              this.props.setResources({trace_freq_sec: 60});
                              this.setState({trace: T("Every 60 Seconds")});
                            }} >
                            {T("Every 60 Seconds")}
                          </Dropdown.Item>
                        </Dropdown.Menu>
                      </Dropdown>
                    </Col>
                  </Form.Group>

                  <Form.Group as={Row}>
                    <Form.Label column sm="3">
                      {T("Urgent")}
                    </Form.Label>
                    <Col sm="8">
                      <Form.Check
                        type="checkbox"
                        checked={resources.urgent || false}
                        label={T("Skip queues and run query urgently")}
                        onChange={e=>this.props.setResources({urgent: e.currentTarget.checked})}
                      />
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
             <Modal.Title>{ T(this.props.paginator.title) }</Modal.Title>
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

    componentDidMount() {
        this.source = CancelToken.source();
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
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
        }, this.source.token).then(response=>{
            this.checkTools(tools);
        }).catch(err=>{
            let data = err.response && err.response.data && err.response.data.message;
            this.setState({current_error: data});
        });
    }

    render() {
        return <>
                 <Modal.Header closeButton>
                   <Modal.Title>{ T(this.props.paginator.title) }</Modal.Title>
                 </Modal.Header>
                 <Modal.Body>
                   {_.map(this.state.checking_tools, (name, idx)=>{
                       let variant = "success";
                       if (name === this.state.current_tool) {
                           variant = "primary";

                           if (this.state.current_error) {
                               return (
                                   <Alert key={idx} variant="danger" className="text-center">
                                     {T("Checking")} { name }: {this.state.current_error}
                                   </Alert>
                               );
                           }
                       }

                       return (
                           <Alert key={idx} variant={variant} className="text-center">
                             {T("Checking")} { name }
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
        client: PropTypes.object,
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
        this.initializeFromBaseFlow();

        // A bit hacky but whatevs...
        const el = document.getElementById("text-filter-column-name-search-for-artifact-input");
        if (el) {
            setTimeout(()=>el.focus(), 100);
        };
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
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
            cpu_limit: request.cpu_limit,
            iops_limit: request.iops_limit,
            timeout: request.timeout,
            progress_timeout: request.progress_timeout,
            max_rows: request.max_rows,
            max_logs: request.max_logs,
            trace_freq_sec: request.trace_freq_sec,
            max_mbytes: Math.round(
                request.max_upload_bytes / 1024 / 1024 * 100) / 100  || undefined,
        };

        this.setState({
            original_flow: this.props.baseFlow,
            resources: resources,
            initialized_from_parent: true,
        });

        if (_.isEmpty(request.artifacts)) {
            this.setState({artifacts:[]});
            return;
        }


        // Resolve the artifacts from the request into a list of descriptors.
        api.post("v1/GetArtifacts",
                {names: request.artifacts}, this.source.token).then(response=>{
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
        // artifact name, value - parameters dict for this artifact.
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
                if (k==="cpu_limit") {
                    spec.cpu_limit = v;
                    return;
                }
                if (k==="max_batch_wait") {
                    spec.max_batch_wait = v;
                    return;
                }
                if (k==="max_batch_rows") {
                    spec.max_batch_rows = v;
                    return;
                }
                if (k==="timeout") {
                    spec.timeout = v;
                    return;
                }

                // If the value is cleared just let the artifact use
                // its own default value and dont mention it in the
                // request at all.
                if (v !== "") {
                    spec.parameters.env.push({key: k, value: v});
                }
            });
            specs.push(spec);
            artifacts.push(item.name);
        });

        let result = {
            artifacts: artifacts,
            specs: specs,
        };

        result.urgent = this.state.resources.urgent;

        if (this.state.resources.ops_per_second) {
            result.ops_per_second = this.state.resources.ops_per_second;
        }

        if (this.state.resources.cpu_limit) {
            result.cpu_limit = this.state.resources.cpu_limit;
        }

        if (this.state.resources.progress_timeout) {
            result.progress_timeout = this.state.resources.progress_timeout;
        }

        if (this.state.resources.iops_limit) {
            result.iops_limit = this.state.resources.iops_limit;
        }

        if (this.state.resources.timeout) {
            result.timeout = this.state.resources.timeout;
        }

        if (this.state.resources.max_rows) {
            result.max_rows = this.state.resources.max_rows;
        }

        if (this.state.resources.max_logs) {
            result.max_logs = this.state.resources.max_logs;
        }

        if (this.state.resources.trace_freq_sec) {
            result.trace_freq_sec = this.state.resources.trace_freq_sec;
        }

        if (this.state.resources.max_mbytes) {
            result.max_upload_bytes = parseInt(
                this.state.resources.max_mbytes * 1024 * 1024);
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

            // These do not work inside text entries.
            NEXT_STEP: "ctrl+shift+right",
            PREV_STEP: "ctrl+shift+left",
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
                          T("Select Artifacts"),
                          T("New Collection: Select Artifacts to collect"))}
                      setArtifacts={this.setArtifacts}
                      setParameters={this.setParameters}
                    />

                    <NewCollectionConfigParameters
                      parameters={this.state.parameters}
                      setParameters={this.setParameters}
                      artifacts={this.state.artifacts}
                      setArtifacts={this.setArtifacts}
                      configureResourceControl={true}
                      paginator={new PaginationBuilder(
                          T("Configure Parameters"),
                          T("New Collection: Configure Parameters"))}
                      />

                    <NewCollectionResources
                      artifacts={this.state.artifacts}
                      resources={this.state.resources}
                      paginator={new PaginationBuilder(
                          T("Specify Resources"),
                          T("New Collection: Specify Resources"))}
                      setResources={this.setResources} />

                    <NewCollectionRequest
                      paginator={new PaginationBuilder(
                          T("Review"),
                          T("New Collection: Review request"))}
                      request={this.prepareRequest()} />

                    <NewCollectionLaunch
                      artifacts={this.state.artifacts}
                      paginator={new PaginationBuilder(
                          T("Launch"),
                          T("New Collection: Launch collection"))}
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
    NewCollectionResources,
    NewCollectionRequest,
    NewCollectionLaunch,
    PaginationBuilder
 }
