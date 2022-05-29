import React from 'react';
import PropTypes from 'prop-types';
import _ from 'lodash';
import Modal from 'react-bootstrap/Modal';
import Form from 'react-bootstrap/Form';
import Button from 'react-bootstrap/Button';
import InputGroup from 'react-bootstrap/InputGroup';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import classNames from "classnames";
import api from '../core/api-service.js';
import BootstrapTable from 'react-bootstrap-table-next';
import axios from 'axios';
import filterFactory from 'react-bootstrap-table2-filter';
import { formatColumns } from "../core/table.js";
import T from '../i8n/i8n.js';

export default class ArtifactsUpload extends React.Component {
    static propTypes = {
        onClose: PropTypes.func.isRequired,
    };

    state = {
        pack_file: null,
        loading: false,
        uploaded: [],
    }

    componentDidMount() {
        this.source = axios.CancelToken.source();
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
    }

    uploadFile = () => {
        if (!this.state.pack_file) {
            return;
        }

        var reader = new FileReader();
        reader.onload = (event) => {
            var request = {
                data: reader.result.split(",")[1],
            };

            this.setState({loading: true});
            api.post("v1/LoadArtifactPack", request,
                     this.source.token).then(response => {
                let uploaded = _.map(response.data.successful_artifacts,
                                     (x, idx)=>{
                                         return {name: x, id: idx};
                                     });
                this.setState({loading:false, uploaded: uploaded});
            });
        };
        reader.readAsDataURL(this.state.pack_file);
    }

    render() {
        let columns = formatColumns([
            {dataField: "name", text: T("Artifact Name"),
             sort: true, filtered: true}
        ]);

        return (
            <>
              <Modal show={true} onHide={this.props.onClose}>
                <Modal.Header closeButton>
                  <Modal.Title>{T("Upload artifacts from a Zip pack")}</Modal.Title>
                </Modal.Header>
                <Modal.Body>
                  { _.isEmpty(this.state.uploaded) ?
                    <Form className="selectable">
                      <InputGroup className="mb-3">
                        <InputGroup.Prepend>
                          <InputGroup.Text
                            className={classNames({"disabled": !this.state.pack_file})}
                            onClick={this.uploadFile}>
                            { this.state.loading ?
                              <FontAwesomeIcon icon="spinner" spin/> :
                              "Upload" }
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
                    </Form>

                    :

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

                  }
              </Modal.Body>
              <Modal.Footer>
                <Button variant="secondary" onClick={this.props.onClose}>
                  {T("Close")}
                </Button>
              </Modal.Footer>
            </Modal>
            </>
        );
    }
};
