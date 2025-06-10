import './tool-viewer.css';

import _ from 'lodash';
import React from 'react';
import PropTypes from 'prop-types';
import {CancelToken} from 'axios';
import Modal from 'react-bootstrap/Modal';
import InputGroup from 'react-bootstrap/InputGroup';
import Button from 'react-bootstrap/Button';
import api from '../core/api-service.jsx';
import Card from 'react-bootstrap/Card';
import Row from 'react-bootstrap/Row';
import Col from 'react-bootstrap/Col';
import Form from 'react-bootstrap/Form';
import T from '../i8n/i8n.jsx';
import Select from 'react-select';
import VeloValueRenderer from '../utils/value.jsx';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';

import classNames from "classnames";

class ResetToolDialog extends React.Component {
    static propTypes = {
        tool: PropTypes.object,
        onClose: PropTypes.func.isRequired,
    };

    componentDidMount = () => {
        this.source = CancelToken.source();
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
    }

    setToolInfo = (tool) => {
        api.post("v1/SetToolInfo", tool,
                 this.source.token).then((response) => {
            this.setState({tool: response.data});
        }).finally(() => {
            this.props.onClose();
        });
    };

    render() {
        return <Modal show={true}
                      enforceFocus={true}
                      scrollable={false}
                      size="lg"
                      dialogClassName="modal-90w"
                      onHide={this.props.onClose}>
                 <Modal.Header closeButton>
                   <Modal.Title>{T("Tool")} {
                       this.props.tool && this.props.tool.name}</Modal.Title>
                 </Modal.Header>
                 <Modal.Body className="tool-viewer">
                   <h1>{T("Confirm tool definition reset")}</h1>
                   {T("This will reset the tool to its original definition")}
                   <VeloValueRenderer value={this.props.tool}/>
                   <Button
                     onClick={x=>this.setToolInfo(this.props.tool)}
                     variant="outline-info">
                     {this.props.tool && this.props.tool.artifact}
                   </Button>
                 </Modal.Body>
                 <Modal.Footer>
                 </Modal.Footer>
               </Modal>;
    }
}

export default class ToolViewer extends React.Component {
    static propTypes = {
        name: PropTypes.string,
        tool_version: PropTypes.string,
        version: PropTypes.number,
    };

    componentDidMount = () => {
        this.source = CancelToken.source();
        this.fetchToolInfo();
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        if (this.props.name !== prevProps.name ||
            this.props.version !== prevProps.version) {
            this.fetchToolInfo();
        }
    }

    fetchToolInfo = (onclose) => {
        api.get("v1/GetToolInfo",
                {name: this.props.name},
               this.source.token).then((response) => {
            this.setState({tool: response.data});
        }).then(onclose);
    }

    state = {
        showDialog: false,
        tool: {},
        tool_file: null,
        remote_url: "",
    }

    acceptUpstreamHash = ()=>{
        // Accepts the upstream hash by updating the expected hash to it.
        let tool = Object.assign({}, this.state.tool);
        tool.expected_hash = tool.invalid_hash;
        tool.materialize = true;
        tool.invalid_hash = "";
        this.setToolInfo(tool);
    }

    uploadFile = () => {
        if (!this.state.tool_file) {
            return;
        }
        this.setState({loading: true});
        api.upload("v1/UploadTool",
                   {file: this.state.tool_file}, this.state.tool).then(response => {
            this.setState({loading:false, tool: response.data});
        });
    }

    setServeUrl = url=>{
        let tool = Object.assign({}, this.state.tool);
        tool.url = this.state.remote_url;

        // Preserve the hash if we already know it. This avoids us
        // having to refetch the file all the time.
        // tool.hash = "";
        tool.filename = "";
        tool.github_project = "";
        tool.serve_locally = false;

        // Do not force a materialize - it is possible that the server
        // has no egress access and it is not possible to materialize
        // the tool from the server.
        tool.materialize = false;
        this.setToolInfo(tool);
    };

    setToolInfo = (tool) => {
        this.setState({inflight: true});
        api.post("v1/SetToolInfo", tool,
                 this.source.token).then((response) => {
            this.setState({tool: response.data});
        }).finally(() => {
            this.fetchToolInfo(()=> this.setState({inflight: false}));
        });
    };

    serve_upstream = () => {
        let tool = Object.assign({}, this.state.tool);
        tool.serve_url = "";
        tool.serve_locally = false;
        this.setToolInfo(tool);
    };

    serve_locally = () => {
        let tool = Object.assign({}, this.state.tool);
        tool.serve_locally = true;
        tool.materialize = true;
        this.setToolInfo(tool);
    };

    calculateHash = () => {
        let tool = Object.assign({}, this.state.tool);
        tool.materialize = true;
        this.setToolInfo(tool);
    };

    redownloadFile = () => {
        let tool = Object.assign({}, this.state.tool);
        tool.hash = "";
        tool.filename = "";
        tool.materialize = true;
        this.setToolInfo(tool);
    };

    render() {
        let tool = this.state.tool || {};
        let cards = [];

        if (tool.serve_locally && !this.state.hide_1) {
            cards.push(
                <Card key={1} className="tool-card ">
                  <Card.Header>
                    {T("Served Locally")}
                    <span className="float-right clickable close-icon"
                          onClick={()=>this.setState({hide_1: true})}
                          data-effect="fadeOut">
                      <FontAwesomeIcon icon="times"/>
                    </span>
                  </Card.Header>
                  <Card.Body>
                    <Card.Text>
                      {T("ToolLocalDesc")}
                    </Card.Text>
                    { tool.url && <Button
                                    disabled={this.state.inflight}
                                    onClick={this.serve_upstream}>
                                    {T("Serve from upstream")}
                                  </Button> }
                  </Card.Body>
                </Card>
            );
        }
        if (!tool.serve_locally && !this.state.hide_2) {
            cards.push(
                <Card key={2} className="tool-card " >
                  <Card.Header>
                    {T("Served from URL")}
                    <span className="float-right clickable close-icon"
                          onClick={()=>this.setState({hide_2: true})}
                          data-effect="fadeOut">
                      <FontAwesomeIcon icon="times"/>
                    </span>
                  </Card.Header>
                  <Card.Body>
                    <Card.Text>
                      {T("ServedFromURL", tool.url)}
                    </Card.Text>
                      { tool.url && <Button
                                      disabled={this.state.inflight}
                                      onClick={this.serve_locally}>
                                      {T("Serve Locally")}
                                    </Button> }

                  </Card.Body>
                </Card> );
        }

        if (tool.github_project && !this.state.hide_3) {
            cards.push(
                <Card key={3} className="tool-card" >
                  <Card.Header>
                    {T("Served from GitHub")}
                    <span className="float-right clickable close-icon"
                          onClick={()=>this.setState({hide_3: true})}
                          data-effect="fadeOut">
                      <FontAwesomeIcon icon="times"/>
                    </span>
                  </Card.Header>
                  <Card.Body>
                    <Card.Text>
                      {T("ServedFromGithub", tool.github_project,
                         tool.github_asset_regex)}
                    </Card.Text>
                      <Button variant="primary"
                              disabled={this.state.inflight}
                              onClick={this.redownloadFile}>{T("Refresh Github")}
                      </Button>

                  </Card.Body>
                </Card>
            );
        };

        if (!tool.hash && !this.state.hide_4) {
            cards.push(
                <Card key={4}  className="tool-card ">
                  <Card.Header>
                    {T("Placeholder Definition")}
                    <span className="float-right clickable close-icon"
                          onClick={()=>this.setState({hide_4: true})}
                          data-effect="fadeOut">
                      <FontAwesomeIcon icon="times"/>
                    </span>
                  </Card.Header>
                  <Card.Body>
                    <Card.Text>
                      {T("PlaceHolder")}
                    </Card.Text>
                      <Button variant="primary"
                              disabled={this.state.inflight}
                              onClick={this.calculateHash}>{T("Materialize Hash")}
                      </Button>

                  </Card.Body>
                </Card>
            );
        }

        if (tool.hash && !this.state.hide_5) {
            cards.push(
                <Card key={5}  className="tool-card ">
                  <Card.Header>
                     {T("Tool Hash Known")}
                    <span className="float-right clickable close-icon"
                          onClick={()=>this.setState({hide_5: true})}
                          data-effect="fadeOut">
                      <FontAwesomeIcon icon="times"/>
                    </span>
                  </Card.Header>
                  <Card.Body>
                    <Card.Text>
                      {T("ToolHash")}
                    </Card.Text>
                      <Button variant="primary"
                              disabled={this.state.inflight}
                              onClick={this.redownloadFile}>{T("Re-Download File")}
                      </Button>

                  </Card.Body>
                </Card>
            );
        }

        if(tool.admin_override && !this.state.hide_6) {
            cards.push(
                <Card key={6}  className="tool-card ">
                  <Card.Header>
                    {T("Admin Override")}
                    <span className="float-right clickable close-icon"
                          onClick={()=>this.setState({hide_6: true})}
                          data-effect="fadeOut">
                      <FontAwesomeIcon icon="times"/>
                    </span>
                  </Card.Header>
                  <Card.Body>
                    <Card.Text>
                      {T("AdminOverride")}
                    </Card.Text>
                  </Card.Body>
                </Card>
            );
        }

        if (!tool.hash && !this.state.url && !this.state.github_project) {
            cards.push(
                <Card key={7}  className="tool-card ">
                  <Card.Header>
                     {T("Error")}
                  </Card.Header>
                  <Card.Body>
                    <Card.Text>
                      {T("ToolError")}
                    </Card.Text>
                  </Card.Body>
                </Card>
            );
        }

        let tool_version_options = _.map(tool.versions, x=>{
             return {value: x.artifact,
                     tool: x,
                     label: x.artifact,
                     isFixed: true};
        });
        return (
            <>
              { this.state.showUpdateDialog &&
                <ResetToolDialog
                  tool={this.state.showUpdateDialog}
                  onClose={x=>this.fetchToolInfo(
                      x=>this.setState({showUpdateDialog: false}))}>
                </ResetToolDialog>
              }
              <Modal show={this.state.showDialog}
                     className="full-height"
                     dialogClassName="modal-90w"
                     enforceFocus={true}
                     scrollable={true}
                     onHide={() => this.setState({showDialog: false})}>
                <Modal.Header closeButton>
                  <Modal.Title>{T("Tool")} {this.props.name}</Modal.Title>
                </Modal.Header>
                <Modal.Body className="tool-viewer">
                  <dl className="row">
                    { tool.versions &&
                      <>
                        <dt  className="col-4">{T("Other Definitions")}</dt>
                        <dd className="col-8">
                          <Select
                            className="tool-selector"
                            classNamePrefix="velo"
                            placeholder={T("Select other definition to reset inventory")}
                            options={tool_version_options}
                            onChange={e=>{
                                e.tool && this.setState({showUpdateDialog: e.tool});
                            }}
                          />
                        </dd>
                      </>}
                    { tool.artifact &&
                      <>
                        <dt className="col-4">{T("Artifact Definition")}</dt>
                        <dd className="col-8">{tool.artifact}</dd></>}

                    { tool.name &&
                      <>
                        <dt className="col-4">{T("Tool Name")}</dt>
                        <dd className="col-8">{tool.name}</dd></>}

                    { tool.version &&
                      <>
                        <dt className="col-4">{T("Tool Version")}</dt>
                        <dd className="col-8">{tool.version}</dd></>}

                    { tool.expected_hash &&
                      <>
                        <dt className="col-4">{T("Expected Hash")}</dt>
                        <dd className="col-8">
                          {tool.expected_hash}
                        </dd></>}

                    { tool.invalid_hash &&
                      <>
                        <dt className="col-4">{T("Upstream Hash")}</dt>
                        <dd className="col-8">
                          {tool.invalid_hash}
                          <Button
                             onClick={x=>this.acceptUpstreamHash()}
                            variant="outline-info">
                            {T("Click to accept")}
                          </Button>
                        </dd></>}

                    { tool.url &&
                      <>
                        <dt className="col-4">{T("Upstream URL")}</dt>
                        <dd className="col-8">{tool.url}</dd> </>}

                    { tool.filename &&
                      <>
                        <dt className="col-4">{T("Endpoint Filename")}</dt>
                        <dd className="col-8">{tool.filename}</dd></>}

                    { tool.hash &&
                      <>
                        <dt className="col-4">{T("Hash")}</dt>
                        <dd className="col-8">{ tool.hash }</dd> </>}

                    { tool.github_project &&
                      <>
                        <dt className="col-4">{T("Github Project")}</dt>
                        <dd className="col-8">{ tool.github_project}</dd></>}

                    { tool.github_asset_regex &&
                      <>
                        <dt className="col-4">{T("Github Asset Regex")}</dt>
                        <dd className="col-8">{ tool.github_asset_regex}</dd></>}

                    { tool.serve_locally &&
                      <>
                        <dt className="col-4">{T("Serve Locally")}</dt>
                        <dd className="col-8">{ tool.serve_locally }</dd></>}

                    { tool.serve_url &&
                      <>
                        <dt className="col-4">{T("Serve URL")}</dt>
                        <dd className="col-8">{ tool.serve_url }</dd></>}

                    { tool.admin_override &&
                      <>
                        <dt className="col-4">{T("Admin Override")}</dt>
                        <dd className="col-8">{ tool.admin_override }</dd></>}
                  </dl>
                  <Row>
                    <Col sm="12">
                      <Card>
                        <Card.Header className="text-center">{T("Override Tool")}</Card.Header>
                        <Card.Body>
                          <Card.Text>
                            {T("OverrideToolDesc")}
                          </Card.Text>
                          <Form className="selectable">
                            <InputGroup className="mb-3 custom-file-button">
                              { this.state.tool_file ?
                                <Form.Label
                                  className={classNames({"foo": "bar","disabled": !this.state.tool_file})}
                                  disabled={!this.state.tool_file}
                                  onClick={this.uploadFile}>
                                  { this.state.loading ?
                                    <FontAwesomeIcon icon="spinner" spin /> :
                                    T("Click to upload file")
                                  }
                                </Form.Label> :

                                <Form.Label data-browse={T("Select file")}>
                                  { this.state.tool_file ? this.state.tool_file.name :
                                    T("Select file")}
                                </Form.Label>
                              }
                              <Form.Control type="file"
                                            onChange={e => {
                                                if (!_.isEmpty(e.currentTarget.files)) {
                                                    this.setState({tool_file: e.currentTarget.files[0]});
                                                }
                                            }}
                              />
                            </InputGroup>
                          </Form>
                          <Form className="selectable">
                            <InputGroup>
                              <Button
                                disabled={this.state.inflight || !this.state.remote_url}
                                onClick={this.setServeUrl}>
                                { this.state.inflight ?
                                  <FontAwesomeIcon icon="spinner" spin /> :
                                  T("Set Serve URL") }
                              </Button>
                              <Form.Control as="input"
                                            value={this.state.remote_url}
                                            onChange={e=>this.setState(
                                                {remote_url: e.currentTarget.value})}
                              />
                            </InputGroup>
                          </Form>
                        </Card.Body>
                      </Card>
                    </Col>
                  </Row>
                  <Row>
                    { _.map(cards, (x, i)=>{
                        return <Col key={i}>{x}</Col>;
                    })}
                  </Row>
                </Modal.Body>
                <Modal.Footer>
                </Modal.Footer>
              </Modal>
              <Button
                className="tool-link"
                onClick={() => this.setState({showDialog: true})}
                variant="outline-info">
                <FontAwesomeIcon icon="external-link-alt"/>
                <span className="button-label">
                  { this.props.name } { this.props.tool_version &&
                                        "(" + this.props.tool_version + ")"  }
                </span>
              </Button>
            </>
        );
    }
};
