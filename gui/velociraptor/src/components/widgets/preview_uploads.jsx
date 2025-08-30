import "./preview_uploads.css";

import _ from 'lodash';

import PropTypes from 'prop-types';
import React, { Component } from 'react';
import {CancelToken} from 'axios';
import api from '../core/api-service.jsx';
import Button from 'react-bootstrap/Button';
import Modal from 'react-bootstrap/Modal';
import Container from  'react-bootstrap/Container';
import HexView from '../utils/hex.jsx';
import SearchHex from './search.jsx';
import Spinner from '../utils/spinner.jsx';
import HexPaginationControl from './pagination.jsx';
import Tab from 'react-bootstrap/Tab';
import Tabs from 'react-bootstrap/Tabs';
import T from '../i8n/i8n.jsx';
import VeloValueRenderer from '../utils/value.jsx';
import Row from 'react-bootstrap/Row';
import Col from 'react-bootstrap/Col';
import Form from 'react-bootstrap/Form';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import Pagination from 'react-bootstrap/Pagination';
import classNames from "classnames";
import Download from "../widgets/download.jsx";

// https://en.wikipedia.org/wiki/List_of_file_signatures
const patterns = [
    ["image/png", [0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A]],
    ["image/jpg", [0xFF, 0xD8, 0xFF, 0xDB]],
    ["image/jpg", [0xFF, 0xD8, 0xFF, 0xE0, "?", "?", 0x4A, 0x46, 0x49, 0x46, 0x00, 0x01]],
    ["image/jpg", [0xFF, 0xD8, 0xFF, 0xE1, "?", "?", 0x45, 0x78, 0x69, 0x66, 0x00, 0x00]],
    ["image/bmp", [66, 77]],
];

function checkMime(buffer) {
    for(let i=0; i<patterns.length; i++) {
        let pattern = patterns[i][1];
        let matched = true;
        for (let j=0; j<pattern.length; j++) {
            // Check the next pattern
            if (j > buffer.length) {
                matched = false;
                break;
            }
            // Skip this check
            if (pattern[j] === "?") {
                continue ;
            }

            if (pattern[j] !== buffer[j]) {
                matched = false;
                break;
            }
        }
        if (matched) {
            return patterns[i][0];
        }
    }
    return "";
}

class TextViewTab extends React.Component {
    static propTypes = {
        params:  PropTypes.object,
        url:     PropTypes.string,
        size:    PropTypes.number,
        base_offset: PropTypes.number,
        setBaseOffset: PropTypes.func,
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
        this.fetchPage_(0);
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        if (!_.isEqual(prevProps.params, this.props.params) ||
            !_.isEqual(prevProps.base_offset, this.props.base_offset) ||
            !_.isEqual(prevState.page, this.state.page) ||
            !_.isEqual(prevState.columns, this.state.columns)) {
            this.fetchPage_(this.state.page);
        };
    }

    fetchPage_ = (page, ondone) => {
        if (this.state.goto_error) {
            return;
        }

        let params = Object.assign({}, this.props.params);

        this.source.cancel();
        this.source = CancelToken.source();

        // read a bit more than we need to so the text view looks a
        // bit more full.
        params.lines = 40;
        params.offset = this.props.base_offset || 0;
        params.text_filter = true;

        api.get_blob(this.props.url, params, this.source.token).then(
            response=>{
                if (response.cancel) return;

                let content_range = response.blob && response.blob.headers &&
                    response.blob.headers["content-range"];
                if(content_range) {
                    const matches = content_range.match(/^(\w+) ((\d+)-(\d+)|\*)\/(\d+|\*)$/);
                    const [, , , , end, size] = matches;
                    this.setState({next_offset: Number(end || 0), total_size: Number(size || 0)});
                }

                const view = new Uint8Array(response.data);
                this.setState({
                    base_offset: params.offset,
                    view: view,
                    version: this.state.version+1,
                    rawdata: this.parseFileContentToTextRepresentation_(view),
                    loading: false});
                if(ondone) {ondone();};
            });
        this.setState({loading: true});
    }

    parseFileContentToTextRepresentation_ = intArray=>{
        let rawdata = "";
        for (var i = 0; i < intArray.length; i++) {
            let c = intArray[i];
            // Skip nulls to compress utf16
            if (c == 0) {
                continue;
            }

            if(c >= 0x20 && c<0x7f ||
               c === 10 || c === 13 || c === 9) {
                rawdata += String.fromCharCode(intArray[i]);
            } else {
                rawdata += ".";
            }
        };
        return rawdata;
    };

    state = {
        rawdata: "",
        next_offset: 0,
        total_size: 0,
    }

    render() {
        return  <Container className="file-hex-view">
                  <Spinner loading={this.state.loading}/>
                  <Row>
                    <Col sm="12">
                      <Pagination className="hex-goto">
                        <Pagination.First
                          disabled={this.props.base_offset===0}
                          onClick={()=>this.props.setBaseOffset(0)}/>
                        <Button
                          variant="outline-info"
                          className="page-link form-control text-paginator"
                          disabled={true}
                          >
                          { this.props.base_offset } - { this.state.next_offset } / { this.state.total_size }
                        </Button>
                        <Form.Control
                          as="input"
                          className={classNames({
                              "page-link": true,
                              "goto-invalid": this.state.goto_error,
                          })}
                          placeholder={T("Goto Offset")}
                          spellCheck="false"
                          value={this.state.goto_offset}
                          onChange={e=> {
                              let goto_offset = e.target.value;
                              let old_goto_offset = this.state.goto_offset;

                              if (goto_offset === "") {
                                  this.props.setBaseOffset(0);
                                  this.setState({goto_offset: "", goto_error: false});
                                  return;
                              }

                              let base_offset = Number(goto_offset);
                              if (isNaN(base_offset)) {
                                  this.setState({goto_offset: old_goto_offset});
                                  return;
                              }

                              if (base_offset > this.state.total_size) {
                                  goto_offset = this.state.total_size;
                                  base_offset = this.state.total_size;
                                  goto_offset = old_goto_offset;
                              }
                              this.props.setBaseOffset(goto_offset);
                              this.setState({goto_offset: goto_offset, goto_error: false});
                          }}/>

                        <Pagination.Next
                          disabled={this.state.next_offset===this.state.total_size}
                          onClick={()=>this.props.setBaseOffset(this.state.next_offset)}/>
                      </Pagination>
                    </Col>
                  </Row>
                  <Row>
                    <Col sm="12" className="hexdump-pane">
                      <div className="panel textdump">
                        {this.state.rawdata}
                      </div>
                    </Col>
                  </Row>
                </Container>;
    };
}

class HexViewTab  extends React.Component {
    static propTypes = {
        params:  PropTypes.object,
        url:     PropTypes.string,
        size:    PropTypes.number,

        // The offset in the file where this screen views
        base_offset: PropTypes.number,
        setBaseOffset: PropTypes.func,
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
        this.fetchPage_(0);
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        if (!_.isEqual(prevProps.params, this.props.params) ||
            !_.isEqual(prevProps.base_offset, this.props.base_offset) ||
            !_.isEqual(prevState.page, this.state.page) ||
            !_.isEqual(prevState.columns, this.state.columns)) {
            this.fetchPage_(this.state.page);
        };
    }

    state = {
        page: 0,
        rows: 25,
        columns: 0x10,
        view: undefined,
        loading: true,
        highlights: {},
        highlight_version: 0,
        version: 0,
    }

    fetchPage_ = (page, ondone) => {
        let params = Object.assign({}, this.props.params);

        this.source.cancel();
        this.source = CancelToken.source();

        params.length = this.state.rows * this.state.columns;
        params.offset = this.props.base_offset;

        api.get_blob(this.props.url, params, this.source.token).then(
            response=>{
                if (response.cancel) return;

                const view = new Uint8Array(response.data);
                this.setState({
                    base_offset: params.offset,
                    view: view,
                    version: this.state.version+1,
                    loading: false});
                if(ondone) {ondone();};
            });
        this.setState({loading: true});
    }

    render() {
        var chunkSize = this.state.rows * this.state.columns;
        let total_size = this.props.size || 0;
        let pageCount = Math.ceil(total_size / chunkSize);

        return (
            <Container className="file-hex-view">
              <Spinner loading={this.state.loading}/>
              <Row>
                <Col sm="4">
                  <HexPaginationControl
                    page_size={chunkSize}
                    total_size={total_size}
                    total_pages={pageCount}
                    current_page={this.state.page}
                    set_highlights={(name, hits)=>{
                        let h = this.state.highlights;
                        h[name] = hits;
                        this.setState({
                            highlights: h,
                            highlight_version: this.state.highlight_version+1});
                    }}
                    onPageChange={page=>{
                        this.fetchPage_(page, ()=>{
                            this.props.setBaseOffset(page * chunkSize);
                            this.setState({
                                page: page,
                            });
                        });
                    }}
                  />
                </Col>
                <Col sm="8" className="hex-goto">
                  <SearchHex
                    base_offset={this.state.base_offset}
                    vfs_components={this.props.params &&
                                    this.props.params.fs_components}
                    page_size={chunkSize}
                    current_page={this.state.page}
                    byte_array={this.state.view}
                    version={this.state.version}
                    onPageChange={page=>{
                        this.fetchPage_(page, ()=>{
                            this.props.setBaseOffset(page * chunkSize);
                            this.setState({
                                page: page,
                            });
                        });
                    }}
                    set_highlights={(name, hits)=>{
                        let h = this.state.highlights;
                        h[name] = hits;
                        this.setState({
                            highlights: h,
                            highlight_version: this.state.highlight_version+1});
                    }}/>
                </Col>
              </Row>
              <Row>
                <Col sm="12" className="hexdump-pane">
                  <div className="panel hexdump">
                    <HexView
            // Highlights on top of the data.
                      highlights={this.state.highlights}
                      highlight_version={this.state.highlight_version}
                      base_offset={this.props.base_offset}
                      height={this.state.rows}
                      rows={this.state.rows}
                      setColumns={v=>this.setState({columns: v})}
                      columns={this.state.columns}

            // The data that will be rendered
                      byte_array={this.state.view}
                      version={this.state.version} />
                  </div>
                </Col>
              </Row>
            </Container>
        );
    }
}

class InspectDialog extends React.Component {
    static propTypes = {
        params:  PropTypes.object,
        url:     PropTypes.string,
        size:    PropTypes.number,
        upload: PropTypes.any,
        onClose: PropTypes.func.isRequired,
    }

    state = {
        tab: "hex",
        base_offset: 0,
    }

    render() {
        let filename = this.props.upload && this.props.upload.Path;
        let components = this.props.upload && this.props.upload.Components;

        return (
            <Modal show={true}
                   dialogClassName="modal-90w"
                   enforceFocus={true}
                   className="full-height"
                   scrollable={true}
                   onHide={this.props.onClose}>
              <Modal.Header closeButton>
                <Modal.Title>Inspect File</Modal.Title>
              </Modal.Header>
              <Modal.Body>
                <Tabs activeKey={this.state.tab}
                      onSelect={tab=>this.setState({tab: tab})}>
                  <Tab eventKey="hex" title={T("Hex")}>
                    { this.state.tab === "hex" &&
                      <HexViewTab params={this.props.params}
                                  url={this.props.url}
                                  base_offset={this.state.base_offset}
                                  setBaseOffset={x=>this.setState({base_offset: x})}
                                  size={this.props.size}/>
                    }
                  </Tab>
                  <Tab eventKey="text" title={T("Text")}>
                    { this.state.tab === "text" &&
                      <TextViewTab  params={this.props.params}
                                    url={this.props.url}
                                    base_offset={this.state.base_offset}
                                    setBaseOffset={x=>this.setState({base_offset: x})}
                                    size={this.props.size}/>
                    }
                  </Tab>
                  <Tab eventKey="details" title={T("Details")}>
                    { this.state.tab === "details" &&
                      <>
                        <Download fs_components={components}
                                  filename={filename}
                                  text={filename}/>

                        <div className="preview-json">
                          <VeloValueRenderer value={this.props.upload}/>
                        </div>
                      </>
                    }
                  </Tab>
                </Tabs>
              </Modal.Body>
              <Modal.Footer>
                <Button variant="secondary" onClick={this.props.onClose}>
                  Close
                </Button>
              </Modal.Footer>
            </Modal>
        );
    }
}


export default class PreviewUpload extends Component {
    static propTypes = {
        env: PropTypes.object,
        upload: PropTypes.any,
    }

    state = {
        page: 0,
        columns: 0x10,
        hexDataRows: [],
        view: undefined,
        showDialog: false,
        error: false,
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
        this.fetchPreview_();
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        if (!_.isEqual(prevProps.upload, this.props.upload)) {
            this.fetchPreview_();
        };

        return _.isEqual(this.state.view, prevState.view) &&
            _.isEqual(this.state.error, prevState.error) ;
    }

    isValidEnv = env=>{
        if(env.hunt_id) return true;
        if(env.notebook_cell_id) return true;
        if(env.client_id && env.flow_id) return true;
        if(env.client_id && env.vfs_components) return true;
        return false;
    }

    fetchPreview_ = () => {
        let upload = this.props.upload;
        if (_.isEmpty(upload)) {
            return;
        }

        let accessor = upload.Accessor || "auto";
        let components = this.props.upload.Components;
        if (_.isUndefined(components)) {
            // No components available - do our best
            let path = (this.props.upload.Path || "");
            if (path.includes("\\")) {
                components = path.split("\\");
            } else {
                components = path.split("/");
            }
        }

        let env = this.props.env || {};
        let client_id = env.client_id;
        let flow_id = env.flow_id;
        if (!this.isValidEnv(env)) {
            return;
        };

        components = components.filter(x=>x);

        if (_.isEmpty(components)) {
            return;
        }

        // First get a small header to figure out what to do.
        let params = {
            offset: 0,
            length: 100,
            padding: this.props.upload.Padding,
            fs_components: normalizeComponentList(
                components, client_id, flow_id, accessor),
            client_id: client_id,
            org_id: window.globals.OrgId || "root",
        };
        let url = 'v1/DownloadVFSFile';

        this.setState({url: url, params: params,
                       error: false, loading: true});

        api.get_blob(url, params, this.source.token).then(
            response=>{
                if (response.cancel) return;
                if(response.data && response.data.error) {
                    this.setState({error: true});

                } else {
                    const view = new Uint8Array(response.data);
                    this.setState({view: view, error: false});
                }
            });
    };

    uintToString = (uintArray) => {
        if (_.isEmpty(uintArray)) {
            uintArray=new Uint8Array("");
        }
        return String.fromCharCode.apply(null, uintArray.slice(0, 25));
    }

    render() {
        if (this.state.error) {
            return <FontAwesomeIcon icon="circle-exclamation" />;
        }

        if (_.isString(this.props.upload)) {
            return <>{this.props.upload}</>;
        }
        if (_.isEmpty(this.props.upload) ||
            !_.isObject(this.props.upload) ||
            _.isEmpty(this.state.view) ||
            !this.props.upload.Size ) {
            return <></>;
        }

        let string_data = this.uintToString(this.state.view);
        if (string_data.length > 20) {
            string_data = string_data.substring(0, 20) + "...";
        }

        // Match the data in case it is an image
        if (checkMime(this.state.view)) {
            let params = {
                client_id: this.props.env.client_id,
                org_id: window.globals.OrgId || "root",

                // Only view first 1mb
                length: 1000000,
            };
            params["fs_components"] = this.state.params.fs_components;
            let url = api.href("/api/" + this.state.url, params);
            string_data = <img className="preview-thumbnail"
                               src={url} alt="preview upload" />;
        }

        return (
            <>
              { this.state.showDialog && this.state.params &&
                <InspectDialog params={this.state.params}
                               url={this.state.url}
                               size={this.props.upload.StoredSize ||
                                     this.props.upload.Size || 0}
                               upload={this.props.upload}
                               onClose={()=>this.setState({showDialog: false})}
                /> }
              <Button className="hex-popup client-link"
                      size="sm"
                      onClick={()=>this.setState({showDialog: true})}
                      variant="outline-info">
                {string_data}
              </Button>
            </>
        );
    }
}

const normalizeComponentList = (components, client_id, flow_id, accessor)=>{
    if (!components) {
        return [accessor];
    }

    switch (components[0]) {
    case "clients":
    case "notebooks":
    case "hunts":
        // It is a filestore path already
        return components;

    case "uploads":
        // It is given relative to the client's flow.
        return ["clients", client_id,
                "collections", flow_id].concat(components);
    }

    return ["clients", client_id, "collections", flow_id,
            "uploads", accessor].concat(components);
};
