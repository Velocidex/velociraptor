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
                    Served Locally
                    <span className="float-right clickable close-icon"
                          onClick={()=>this.setState({hide_1: true})}
                          data-effect="fadeOut">
                      <FontAwesomeIcon icon="times"/>
                    </span>
                  </Card.Header>
                  <Card.Body>
                    <Card.Text>
                      Tool will be served from the Velociraptor server
                      to clients if needed. The client will
                      cache the tool on its own disk and compare the hash next
                      time it is needed. Tools will only be downloaded if their
                      hash has changed.
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
                    Served from URL
                    <span className="float-right clickable close-icon"
                          onClick={()=>this.setState({hide_2: true})}
                          data-effect="fadeOut">
                      <FontAwesomeIcon icon="times"/>
                    </span>
                  </Card.Header>
                  <Card.Body>
                    <Card.Text>
                        Clients will fetch the tool directly from
                        <a href={api.base_path + tool.url}>{tool.url}</a> if
                        needed. Note that if the hash does not match the
                        expected hash the clients will reject the file.
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
                    Served from GitHub
                    <span className="float-right clickable close-icon"
                          onClick={()=>this.setState({hide_3: true})}
                          data-effect="fadeOut">
                      <FontAwesomeIcon icon="times"/>
                    </span>
                  </Card.Header>
                  <Card.Body>
                    <Card.Text>
                     Tool URL will be refreshed from
                        GitHub as the latest release from the project
                        <b>{tool.github_project}</b> that matches
                        <b>{tool.github_asset_regex}</b>
                    </Card.Text>
                      <Button variant="primary"
                              disabled={this.state.inflight}
                              onClick={this.redownloadFile}>Refresh Github
                      </Button>

                  </Card.Body>
                </Card>
            );
        };

        if (!tool.hash && !this.state.hide_4) {
            cards.push(
                <Card key={4}>
                  <Card.Header>
                    Placeholder Definition
                    <span className="float-right clickable close-icon"
                          onClick={()=>this.setState({hide_4: true})}
                          data-effect="fadeOut">
                      <FontAwesomeIcon icon="times"/>
                    </span>
                  </Card.Header>
                  <Card.Body>
                    <Card.Text>
                        Tool hash is currently unknown. The first time the tool
                        is needed, Velociraptor will download it from it's
                        upstream URL and calculate its hash.
                    </Card.Text>
                      <Button variant="primary"
                              disabled={this.state.inflight}
                              onClick={this.calculateHash}>Materialize Hash
                      </Button>

                  </Card.Body>
                </Card>
            );
        }

        if (tool.hash && !this.state.hide_5) {
            cards.push(
                <Card key={5}>
                  <Card.Header>
                    Tool Hash Known
                    <span className="float-right clickable close-icon"
                          onClick={()=>this.setState({hide_5: true})}
                          data-effect="fadeOut">
                      <FontAwesomeIcon icon="times"/>
                    </span>
                  </Card.Header>
                  <Card.Body>
                    <Card.Text>
                        Tool hash has been calculated. When clients need to use
                        this tool they will ensure this hash matches what they
                        download.
                    </Card.Text>
                      <Button variant="primary"
                              disabled={this.state.inflight}
                              onClick={this.redownloadFile}>Re-Download File
                      </Button>

                  </Card.Body>
                </Card>
            );
        }

        if(tool.admin_override && !this.state.hide_6) {
            cards.push(
                <Card key={6}>
                  <Card.Header>
                    Admin Override
                    <span className="float-right clickable close-icon"
                          onClick={()=>this.setState({hide_6: true})}
                          data-effect="fadeOut">
                      <FontAwesomeIcon icon="times"/>
                    </span>
                  </Card.Header>
                  <Card.Body>
                    <Card.Text>
                      Tool was manually uploaded by an
                        admin - it will not be automatically upgraded on the
                        next Velociraptor server update.
                    </Card.Text>
                  </Card.Body>
                </Card>
            );
        }

        if (!tool.hash && !this.state.url && !this.state.github_project) {
            cards.push(
                <Card key={7}>
                  <Card.Header>
                    Error
                  </Card.Header>
                  <Card.Body>
                    <Card.Text>
                        Tool's hash is not known and no URL
                        is defined. It will be impossible to use this tool in an
                        artifact because Velociraptor is unable to resolve it. You
                        can manually upload a file.
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
                  <Modal.Title>Tool {this.props.name}</Modal.Title>
                </Modal.Header>
                <Modal.Body className="tool-viewer">
                  <dl className="row">
                    { tool.name &&
                      <>
                        <dt className="col-4">Tool Name</dt>
                        <dd className="col-8">{tool.name}</dd></>}

                    { tool.url &&
                      <>
                        <dt className="col-4">Upstream URL</dt>
                        <dd className="col-8">{tool.url}</dd> </>}

                    { tool.filename &&
                      <>
                        <dt className="col-4">Enpoint Filename</dt>
                        <dd className="col-8">{tool.filename}</dd></>}

                    { tool.hash &&
                      <>
                        <dt className="col-4">Hash</dt>
                        <dd className="col-8">{ tool.hash }</dd> </>}

                    { tool.github_project &&
                      <>
                        <dt className="col-4">Github Project</dt>
                        <dd className="col-8">{ tool.github_project}</dd></>}

                    { tool.github_asset_regex &&
                      <>
                        <dt className="col-4">Github Asset Regex</dt>
                        <dd className="col-8">{ tool.github_asset_regex}</dd></>}

                    { tool.serve_locally &&
                      <>
                        <dt className="col-4">Serve Locally</dt>
                        <dd className="col-8">{ tool.serve_locally }</dd></>}

                    { tool.serve_url &&
                      <>
                        <dt className="col-4">Serve URL</dt>
                        <dd className="col-8">{ tool.serve_url }</dd></>}

                    { tool.admin_override &&
                      <>
                        <dt className="col-4">Admin Override</dt>
                        <dd className="col-8">{ tool.admin_override }</dd></>}
                  </dl>
                  <CardDeck>
                    <Card>
                      <Card.Header className="text-center bg-success">Override Tool</Card.Header>
                      <Card.Body>
                        <Card.Text>
                          As an admin you can manually upload a
                          binary to be used as that tool. This will override the
                          upstream URL setting and provide your tool to all
                          artifacts that need it. Alternative, set a URL for clients
                          to fetch tools from.
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
                              <Form.File.Label data-browse="Select file">
                                { this.state.tool_file ? this.state.tool_file.name :
                                  "Click to upload file"}
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
                                  "Set Serve URL" }
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
