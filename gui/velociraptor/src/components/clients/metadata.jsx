import _ from 'lodash';

import "./metadata.css";
import React, { Component } from 'react';
import PropTypes from 'prop-types';
import {CancelToken} from 'axios';
import api from '../core/api-service.jsx';
import BootstrapTable from 'react-bootstrap-table-next';
import cellEditFactory, { Type } from 'react-bootstrap-table2-editor';
import filterFactory from 'react-bootstrap-table2-filter';
import T from '../i8n/i8n.jsx';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Button from 'react-bootstrap/Button';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { formatColumns } from "../core/table.jsx";


const POLL_TIME = 5000;


export default class MetadataEditor extends Component {
    static propTypes = {
        client_id: PropTypes.string,
        valueRenderer: PropTypes.func,
    }

    state = {
        metadata: {},
        metadata_loading: false,
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
        this.interval = setInterval(this.fetchMetadata, POLL_TIME);
        this.fetchMetadata();
    }

    componentWillUnmount() {
        this.source.cancel();
        clearInterval(this.interval);
    }


    fetchMetadata = () => {
        let client_id = this.props.client_id;
        if (!client_id) {
            return;
        }
        this.setState({metadata_loading: true});

        this.source.cancel();
        this.source = CancelToken.source();

        api.get("v1/GetClientMetadata/" + this.props.client_id,
                {}, this.source.token).then(response=>{
                    if (response.cancel) return;

                    this.setState({metadata: response.data,
                                   metadata_loading: false});
                });
    }

    setMetadata = (row, remove_keys) => {
        var params = {
            client_id: this.props.client_id,
            remove: remove_keys,
            add: [{key: row.key, value: row.value}]};
        api.post("v1/SetClientMetadata", params, this.source.token).then(() => {
            this.fetchMetadata();
        });
    }

    deleteMetadata = (key) => {
        var params = {
            client_id: this.props.client_id,
            remove: [key],
        };
        api.post("v1/SetClientMetadata", params, this.source.token).then(() => {
            this.fetchMetadata();
        });
    }

    render() {
        let columns = formatColumns([{
            dataField: "_id",
            text: "",
            style: {
                width: '8%',
            },
            formatter: (cell, row) => {
                return <ButtonGroup>
                <Button variant="default-outline" size="sm"
                        onClick={() => {
                            // Drop th current row at the current row index.
                            this.deleteMetadata(row.key);
                        }}
                >
                  <FontAwesomeIcon icon="trash"/>
                </Button>
              </ButtonGroup>;
            },
        }, {
            dataField: "key",
            sort: true,
            editable: true,
            filtered: true,
            classes: "metadata-key",
            text: T("Key"),
        }, {
            dataField: "value",
            editable: true,
            sort: true,
            formatter: this.props.valueRenderer,
            text: T("Value"),
        }]);

        columns[0].headerFormatter = (column, colIndex) => {
            if (colIndex === 0) {
                return <ButtonGroup>
                         <Button variant="default-outline" size="sm"
                                 onClick={() => {
                                     this.setMetadata({
                                         key: T(" New Key"),
                                         value: T("New Value")});
                                 }}
                         >
                           <FontAwesomeIcon icon="plus"/>
                         </Button>
                       </ButtonGroup>;
            };
            return column;
        };

        columns[1].editor = {type: Type.TEXTAREA};
        columns[2].editor = {type: Type.TEXTAREA};

        let data = _.map(this.state.metadata.items, (x, idx)=>{
            return {_id: idx, key: x.key || "" , value: x.value || ""};
        });

        return (
            <BootstrapTable
              hover condensed bootstrap4
              data={data}
              keyField="_id"
              headerClasses="alert alert-secondary"
              columns={columns}
              defaultSorted={[{dataField: "key", order:"asc"}]}
              filter={ filterFactory() }
              cellEdit={ cellEditFactory({
                  mode: 'click',
                  afterSaveCell: (oldValue, newValue, row, column) => {
                      if (oldValue === newValue) return;

                      // We are changing the key value, remove the
                      // old key and store the new value.
                      if (column.dataField == "key") {
                          this.setMetadata(row, [oldValue]);
                          return;
                      }

                      // We are changing the value
                      this.setMetadata(row);
                  },
                  blurToSave: true,
              }) }
            />
        );
    }
}
