import './tool-viewer.css';

import _ from 'lodash';
import React from 'react';
import PropTypes from 'prop-types';
import axios from 'axios';
import Modal from 'react-bootstrap/Modal';
import Button from 'react-bootstrap/Button';
import api from '../core/api-service.js';
import Card from 'react-bootstrap/Card';
import CardDeck from 'react-bootstrap/CardDeck';
import Form from 'react-bootstrap/Form';
import InputGroup from 'react-bootstrap/InputGroup';
import T from '../i8n/i8n.js';

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';

import classNames from "classnames";

export default class ToolViewer extends React.Component {
    static propTypes = {
        name: PropTypes.string,
    };

    componentDidMount = () => {
        this.source = axios.CancelToken.source();
        this.fetchToolInfo();
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        if (this.props.name !== prevProps.name) {
            this.fetchToolInfo();
        }
    }

    fetchToolInfo = () => {
        api.get("v1/GetToolInfo",
                {name: this.props.name},
               this.source.token).then((response) => {
            this.setState({tool: response.data});
        });
    }

    state = {
        showDialog: false,
        tool: {},
        tool_file: null,
        remote_url: "",
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
        tool.hash = "";
        tool.filename = "";
        tool.github_project = "";
        tool.serve_locally = false;
        tool.materialize = true;
        this.setToolInfo(tool);
    };

    setToolInfo = (tool) => {
        this.setState({inflight: true});
        api.post("v1/SetToolInfo", tool,
                 this.source.token).then((response) => {
            this.setState({tool: response.data});
        }).finally(() => {
            this.setState({inflight: false});
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
                <Card key={1}>
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
                                    Serve from upstream</Button> }
                  </Card.Body>
                </Card>
            );
        }
        if (!tool.serve_locally && !this.state.hide_2) {
            cards.push(
                <Card key={2}>
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
                      {T("ServedFromURL", api.base_path, tool.url)}
                    </Card.Text>
                      { tool.url && <Button
                                      disabled={this.state.inflight}
                                      onClick={this.serve_locally}>
                                      Serve Locally</Button> }

                  </Card.Body>
                </Card> );
        }

        if (tool.github_project && !this.state.hide_3) {
            cards.push(
                <Card key={3}>
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
                <Card key={4}>
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
                <Card key={5}>
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
                <Card key={6}>
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
                <Card key={7}>
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


        return (
            <>
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
                    { tool.name &&
                      <>
                        <dt className="col-4">{T("Tool Name")}</dt>
                        <dd className="col-8">{tool.name}</dd></>}

                    { tool.url &&
                      <>
                        <dt className="col-4">{T("Upstream URL")}</dt>
                        <dd className="col-8">{tool.url}</dd> </>}

                    { tool.filename &&
                      <>
                        <dt className="col-4">{T("Enpoint Filename")}</dt>
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
                  <CardDeck>
                    <Card>
                      <Card.Header className="text-center">{T("Override Tool")}</Card.Header>
                      <Card.Body>
                        <Card.Text>
                          {T("OverrideToolDesc")}
                        </Card.Text>
                        <Form className="selectable">
                          <InputGroup className="mb-3">
                            <InputGroup.Prepend>
                              <InputGroup.Text
                                className={classNames({"disabled": !this.state.tool_file})}
                                onClick={this.uploadFile}>
                                { this.state.loading ?
                                  <FontAwesomeIcon icon="spinner" spin /> :
                                  "Upload" }
                              </InputGroup.Text>
                            </InputGroup.Prepend>
                            <Form.File custom>
                              <Form.File.Input
                                onChange={e => {
                                    if (!_.isEmpty(e.currentTarget.files)) {
                                        this.setState({tool_file: e.currentTarget.files[0]});
                                    }
                                }}
                              />
                              <Form.File.Label data-browse={T("Select file")}>
                                { this.state.tool_file ? this.state.tool_file.name :
                                  T("Click to upload file")}
                              </Form.File.Label>
                            </Form.File>
                          </InputGroup>
                        </Form>
                        <Form className="selectable">
                          <InputGroup>
                            <InputGroup.Prepend>
                              <InputGroup.Text  as="button"
                                disabled={this.state.inflight || !this.state.remote_url}
                                onClick={this.setServeUrl}>
                                { this.state.inflight ?
                                  <FontAwesomeIcon icon="spinner" spin /> :
                                  T("Set Serve URL") }
                              </InputGroup.Text>
                            </InputGroup.Prepend>
                            <Form.Control as="input"
                                          value={this.state.remote_url}
                                          onChange={e=>this.setState(
                                              {remote_url: e.currentTarget.value})}
                            />
                          </InputGroup>
                        </Form>
                      </Card.Body>
                    </Card>
                  </CardDeck>
                  <CardDeck>
                    { cards }
                  </CardDeck>
                </Modal.Body>
                <Modal.Footer>
                </Modal.Footer>
              </Modal>
              <Button
                className="tool-link"
                onClick={() => this.setState({showDialog: true})}
                variant="outline-info">
                <FontAwesomeIcon icon="external-link-alt"/>
                <span className="button-label">{ this.props.name }</span>
              </Button>
            </>
        );
    }
};
