import React from 'react';
import PropTypes from 'prop-types';
import _ from 'lodash';
import Modal from 'react-bootstrap/Modal';
import Form from 'react-bootstrap/Form';
import Row from 'react-bootstrap/Row';
import Col from 'react-bootstrap/Col';
import Button from 'react-bootstrap/Button';
import InputGroup from 'react-bootstrap/InputGroup';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import classNames from "classnames";
import api from '../core/api-service.jsx';
import BootstrapTable from 'react-bootstrap-table-next';
import {CancelToken} from 'axios';
import filterFactory from 'react-bootstrap-table2-filter';
import { formatColumns } from "../core/table.jsx";
import T from '../i8n/i8n.jsx';

export default class ArtifactsUpload extends React.Component {
    static propTypes = {
        onClose: PropTypes.func.isRequired,
    };

    state = {
        pack_file: null,
        prefix: "",
        loading: false,
        uploaded: [],
        filter: "",

        // after upload the server will cache the file here.
        vfs_path: [],
    }

    componentDidMount() {
        this.source = CancelToken.source();
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        if(!_.isEqual(prevState.filter, this.state.filter) ||
           !_.isEqual(prevState.prefix, this.state.prefix) ||
           !_.isEqual(prevState.vfs_path, this.state.vfs_path)) {
            this.updateFile();
            return true;
        }
        return false;
    }

    // The first call uploads the file and records the cached
    // filestore path
    uploadFile = () => {
        if (!this.state.pack_file) {
            return;
        }

        var reader = new FileReader();
        reader.onload = (event) => {
            var request = {
                prefix: this.state.prefix,
                filter: this.state.filter,
                data: reader.result.split(",")[1],
            };

            this.setState({loading: true});
            api.post("v1/LoadArtifactPack", request,
                     this.source.token).then(response => {
                         if (response.data.cancel) {
                             return ;
                         }

                         let uploaded = _.map(
                             response.data.successful_artifacts,
                             (x, idx)=>{
                                 return {name: x, id: idx};
                             });
                         this.setState({loading:false,
                                        vfs_path: response.data.vfs_path,
                                        uploaded: uploaded});
                     });
        };
        reader.readAsDataURL(this.state.pack_file);
    }

    updateFile = () => {
        if (this.state.loading) {
            return;
        }

        var request = {
            prefix: this.state.prefix,
            filter: this.state.filter,
            vfs_path: this.state.vfs_path,
        };
        api.post("v1/LoadArtifactPack", request,
                 this.source.token).then(response => {
                     if (response.data.cancel) {
                         return ;
                     }
                     let uploaded = _.map(
                         response.data.successful_artifacts,
                         (x, idx)=>{
                             return {name: x, id: idx};
                         });

                     this.setState({loading:false,
                                    vfs_path: response.data.vfs_path,
                                    uploaded: uploaded});
                 });
    };

    importFile = () => {
        var request = {
            prefix: this.state.prefix,
            filter: this.state.filter,
            vfs_path: this.state.vfs_path,
            really_do_it: true,
        };
        api.post("v1/LoadArtifactPack", request,
                 this.source.token).then(response => {
                     if (response.data.cancel) {
                         return ;
                     }
                     this.props.onClose();
                 });
    };

    render() {
        let columns = formatColumns([
            {dataField: "name", text: T("Artifact Name"),
             sort: true, filtered: true}
        ]);

        return (
            <>
              <Modal show={true}
                     dialogClassName="modal-90w"
                     onHide={this.props.onClose}>
                <Modal.Header closeButton>
                  <Modal.Title>{T("Upload artifacts from a Zip pack")}</Modal.Title>
                </Modal.Header>
                <Modal.Body>
                  <Form className="selectable">
                    <Form.Group as={Row}>
                      <Col sm="12">
                        <InputGroup className="full-width">
                          <InputGroup.Prepend>
                            <InputGroup.Text
                              className={classNames({"disabled": !this.state.pack_file})}
                              onClick={()=>this.uploadFile()}>
                              { this.state.loading ?
                                <FontAwesomeIcon icon="spinner" spin/> :
                                T("Upload") }
                            </InputGroup.Text>
                          </InputGroup.Prepend>
                          <Form.File
                            label={T("Select artifact pack (Zip file with YAML definitions)")}
                            custom>
                            <Form.File.Input
                              onChange={e => {
                                  if (!_.isEmpty(e.currentTarget.files)) {
                                      this.setState({pack_file: e.currentTarget.files[0]});
                                  }
                              }}
                            />
                            <Form.File.Label data-browse="Select file">
                              { this.state.pack_file ? this.state.pack_file.name :
                                T("Click to upload artifact pack file")}
                            </Form.File.Label>
                          </Form.File>
                        </InputGroup>
                      </Col>
                    </Form.Group>
                  </Form>

                  { !_.isEmpty(this.state.vfs_path) &&
                    <Form>
                      <Form.Group as={Row}>
                        <Form.Label column sm="3">{T("Prefix")}</Form.Label>
                        <Col sm="8">
                          <Form.Control as="input" rows={3}
                                        placeholder={T("Append prefix to all artifact names")}
                                        spellCheck="false"
                                        value={this.state.prefix}
                                        onChange={e => {
                                            this.setState({prefix: e.target.value});
                                        }}
                          />
                        </Col>
                      </Form.Group>
                      <Form.Group as={Row}>
                        <Form.Label column sm="3">{T("Filter")}</Form.Label>
                        <Col sm="8">
                          <Form.Control as="input"
                                        placeholder={T("Filter name of artifacts to load")}
                                        spellCheck="false"
                                        value={this.state.filter}
                                        onChange={e => {
                                            this.setState({filter: e.target.value});
                                        }}
                          />
                        </Col>
                      </Form.Group>

                      <BootstrapTable
                        hover
                        condensed
                        keyField="id"
                        bootstrap4
                        headerClasses="alert alert-secondary"
                        bodyClasses="fixed-table-body"
                        data={this.state.uploaded}
                        columns={columns}
                        filter={ filterFactory() }
                      />
                    </Form> }
                </Modal.Body>
                <Modal.Footer>
                  <Button variant="secondary" onClick={this.props.onClose}>
                    {T("Cancel")}
                  </Button>
                <Button variant="secondary"
                        disabled={_.isEmpty(this.state.uploaded)}
                        onClick={()=>this.importFile()}>
                  {T("Import Artifacts", this.state.uploaded && this.state.uploaded.length)}
                </Button>
              </Modal.Footer>
            </Modal>
            </>
        );
    }
};
